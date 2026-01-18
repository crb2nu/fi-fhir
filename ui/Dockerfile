# Build stage
FROM node:22-alpine AS builder

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
FROM nginx:alpine

# Copy built assets
COPY --from=builder /app/build /usr/share/nginx/html

# Custom nginx config for SPA routing
COPY nginx/default.conf /etc/nginx/conf.d/default.conf

EXPOSE 3000

# Run as non-root
RUN chown -R nginx:nginx /usr/share/nginx/html && \
    chown -R nginx:nginx /var/cache/nginx && \
    chown -R nginx:nginx /var/log/nginx && \
    touch /var/run/nginx.pid && \
    chown nginx:nginx /var/run/nginx.pid

USER nginx

CMD ["nginx", "-g", "daemon off;"]
