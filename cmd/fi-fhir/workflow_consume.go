package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v3"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventbus"
)

type workflowConsumerConfig struct {
	Driver       string            `yaml:"driver"`
	Subscription string            `yaml:"subscription"`
	Options      map[string]string `yaml:"options"`
}

func loadWorkflowConsumer(path string) (*workflowConsumerConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("backend configuration exceeds 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config workflowConsumerConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, errors.New("invalid backend configuration YAML")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("backend configuration must contain one document")
	}
	if config.Driver == "" || config.Subscription == "" {
		return nil, errors.New("backend requires driver and subscription")
	}
	for key, value := range config.Options {
		var missing bool
		config.Options[key] = os.Expand(value, func(name string) string {
			value, ok := os.LookupEnv(name)
			if !ok {
				missing = true
			}
			return value
		})
		if missing {
			return nil, fmt.Errorf("backend option %s references an unset environment variable", key)
		}
	}
	return &config, nil
}

func runWorkflowConsume(args []string) (retErr error) {
	flags := flag.NewFlagSet("workflow consume", flag.ContinueOnError)
	workflowPath := flags.String("config", "", "Workflow YAML file")
	backendPath := flags.String("backend", "", "Backend YAML file (driver, subscription, options)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *workflowPath == "" || *backendPath == "" || flags.NArg() != 0 {
		return errors.New("usage: fi-fhir workflow consume --config workflow.yaml --backend backend.yaml")
	}
	config, err := loadWorkflowConsumer(*backendPath)
	if err != nil {
		return err
	}
	w, err := workflow.LoadWorkflow(*workflowPath)
	if err != nil {
		return err
	}
	if issues := w.Validate(); len(issues) > 0 {
		return fmt.Errorf("invalid workflow: %w", errors.Join(issues...))
	}
	validation, err := workflow.ValidateWorkflow(w)
	if err != nil {
		return err
	}
	if !validation.Valid {
		return errors.New("invalid workflow; run fi-fhir workflow validate for diagnostics")
	}
	engine, err := workflow.NewEngine(w)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backend, err := eventbus.Open(ctx, config.Driver, config.Options)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, backend.Close(), workflow.GetQueueRegistry().Close()) }()
	err = backend.Consume(ctx, config.Subscription, engine.EventHandler())
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
