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
# Browser tab title, inlined into index.html by vite build (see src/brand).
ARG VITE_BRAND_TITLE=
ENV VITE_BRAND_TITLE=${VITE_BRAND_TITLE}
RUN npm run build

# --- Runtime stage ---
FROM nginx:stable-alpine AS runtime

# Static assets
COPY --from=build /app/dist /usr/share/nginx/html

# Site config: /api/ reverse proxy to the backend + SPA fallback
COPY nginx/default.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
