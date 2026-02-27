FROM nginx:alpine

# Install envsubst (part of gettext) for environment variable substitution
RUN apk add --no-cache gettext

# Copy static frontend build output into nginx html directory
COPY build /usr/share/nginx/html

# Copy nginx config template and entrypoint script
COPY nginx.conf.template /etc/nginx/conf.d/default.conf.template
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

EXPOSE 80

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
