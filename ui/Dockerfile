# Build stage
#
# NOTE: build args used in `FROM ...` must be declared before the first `FROM`
# to be available for all stages.
ARG NODE_IMAGE=node:22-alpine
ARG NGINX_IMAGE=nginx:alpine
FROM ${NODE_IMAGE} AS builder

WORKDIR /app

# Copy package files first for better layer caching
COPY package.json package-lock.json ./
RUN npm ci

# Copy source code
COPY . .

# Optional UI build metadata (injected by CI)
ARG VITE_BUILD_SHA
ARG VITE_BUILD_TAG
ARG VITE_BUILD_TIME
ENV VITE_BUILD_SHA=$VITE_BUILD_SHA
ENV VITE_BUILD_TAG=$VITE_BUILD_TAG
ENV VITE_BUILD_TIME=$VITE_BUILD_TIME

# Build the production bundle
RUN npm run build

# Production stage - lightweight nginx to serve static files
FROM ${NGINX_IMAGE}

# Default backend origin for reverse-proxying API routes from the UI container.
# Override at runtime to match your deployment.
ENV FI_FHIR_UI_API_ORIGIN=http://fi-fhir

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
