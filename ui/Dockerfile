# Build stage
#
# NOTE: build args used in `FROM ...` must be declared before the first `FROM`
# to be available for all stages.
ARG NODE_IMAGE=node:22-alpine
ARG NPM_VERSION=10.9.3
ARG NGINX_IMAGE=nginx:1.31.2-alpine-slim@sha256:dd722b8ee8794f3c273bfaf8b5351b0652a68ccd73c17e5f0d029857a58f25ef
FROM ${NODE_IMAGE} AS builder
ARG NPM_VERSION

WORKDIR /app

# Copy package files first for better layer caching
COPY package.json package-lock.json ./
RUN npm install --global "npm@${NPM_VERSION}" && \
    npm ci --no-audit --no-fund

# Copy source code
COPY . .

# Optional UI build metadata (injected by CI)
ARG VITE_BUILD_SHA
ARG VITE_BUILD_TAG
ARG VITE_BUILD_TIME
ARG VITE_FI_FHIR_PREVIEW_INTEGRATION_ID=adt-east
ARG VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=false
ENV VITE_BUILD_SHA=$VITE_BUILD_SHA
ENV VITE_BUILD_TAG=$VITE_BUILD_TAG
ENV VITE_BUILD_TIME=$VITE_BUILD_TIME
ENV VITE_FI_FHIR_PREVIEW_INTEGRATION_ID=$VITE_FI_FHIR_PREVIEW_INTEGRATION_ID
ENV VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=$VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED

# Build the production bundle
RUN npm run build

# Production stage - lightweight nginx to serve static files
FROM ${NGINX_IMAGE}

# Default backend origin for reverse-proxying API routes from the UI container.
# Override at runtime to match your deployment.
ENV FI_FHIR_UI_API_ORIGIN=http://fi-fhir-api:8081

# Copy built assets
COPY --from=builder /app/build /usr/share/nginx/html

# Custom nginx config template (rendered via nginx entrypoint envsubst)
COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template

EXPOSE 3000

# Run as non-root
RUN chown -R nginx:nginx /usr/share/nginx/html && \
    chown -R nginx:nginx /etc/nginx/conf.d && \
    chown -R nginx:nginx /etc/nginx/templates && \
    chown -R nginx:nginx /var/cache/nginx && \
    chown -R nginx:nginx /var/log/nginx && \
    touch /var/run/nginx.pid && \
    chown nginx:nginx /var/run/nginx.pid

USER nginx

CMD ["nginx", "-g", "daemon off;"]
