# Factory Showroom - static demo application for Brunernetes.
# Single stage: nothing to compile, the site is plain HTML, CSS and JS.
FROM nginx:1.27-alpine

LABEL org.opencontainers.image.title="Factory Showroom" \
      org.opencontainers.image.description="Static English-only animated CV and DevOps portfolio page with a four stage assembly line. Demo application for Brunernetes." \
      org.opencontainers.image.version="0.1.0" \
      org.opencontainers.image.authors="Bruner" \
      org.opencontainers.image.vendor="Brunernetes" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="https://github.com/bruner/brunernetes" \
      org.opencontainers.image.documentation="https://github.com/bruner/brunernetes/tree/main/demo/factory-showroom" \
      org.opencontainers.image.base.name="docker.io/library/nginx:1.27-alpine" \
      dev.bruner.component="demo-application" \
      dev.bruner.stage="showroom"

# Site content and configuration.
COPY nginx.conf /etc/nginx/nginx.conf
COPY site/ /usr/share/nginx/html/

# Writable temp dirs for the unprivileged nginx user, correct ownership everywhere.
RUN set -eux; \
    mkdir -p /tmp/client_temp /tmp/proxy_temp /tmp/fastcgi_temp /tmp/uwsgi_temp /tmp/scgi_temp; \
    chown -R 101:101 /tmp/client_temp /tmp/proxy_temp /tmp/fastcgi_temp /tmp/uwsgi_temp /tmp/scgi_temp \
        /usr/share/nginx/html /var/cache/nginx; \
    nginx -t -c /etc/nginx/nginx.conf

# uid 101 is the "nginx" user in the official alpine image.
USER 101:101

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://127.0.0.1:8080/healthz || exit 1

STOPSIGNAL SIGQUIT

CMD ["nginx", "-g", "daemon off;"]
