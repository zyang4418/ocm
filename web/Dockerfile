# --- Build stage ---
FROM node:24-alpine AS build

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci || npm install

COPY . .
# Optional ICP filing number, inlined into the bundle by vite build.
ARG VITE_ICP_NUMBER=
ENV VITE_ICP_NUMBER=${VITE_ICP_NUMBER}
# External docs site origin; inlined into the bundle by vite build.
ARG VITE_DOCS_URL=
ENV VITE_DOCS_URL=${VITE_DOCS_URL}
# Public key fingerprint pinned at build time for login password encryption.
ARG VITE_SM2_PUBLIC_KEY_FINGERPRINT=
ENV VITE_SM2_PUBLIC_KEY_FINGERPRINT=${VITE_SM2_PUBLIC_KEY_FINGERPRINT}
RUN npm run build

# --- Runtime stage ---
FROM nginx:stable-alpine AS runtime

# Static assets
COPY --from=build /app/dist /usr/share/nginx/html

# SPA fallback so client-side routes work
RUN printf 'server {\n\
  listen 80;\n\
  server_name _;\n\
  root /usr/share/nginx/html;\n\
  index index.html;\n\
  resolver 127.0.0.11 valid=5s;\n\
  location /api/ {\n\
    set $upstream http://backend:8080;\n\
    proxy_pass $upstream;\n\
    proxy_set_header Host $host;\n\
    proxy_set_header X-Real-IP $remote_addr;\n\
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n\
    # SSE (AI 助手流式回答) 需要关闭响应缓冲并保持长连接。\n\
    proxy_http_version 1.1;\n\
    proxy_set_header Connection "";\n\
    proxy_buffering off;\n\
  }\n\
  location / {\n\
    try_files $uri $uri/ /index.html;\n\
  }\n\
}\n' > /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
