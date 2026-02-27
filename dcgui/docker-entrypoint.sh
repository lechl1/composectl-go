#!/bin/sh
set -e

# Default to http://dcapi if DC_API_URL not provided (safe fallback for compose setups)
: "${DC_API_URL:=http://dcapi}"
export DC_API_URL

# Default AUTH_DISABLED to false unless provided
: "${AUTH_DISABLED:=false}"
export AUTH_DISABLED

# Render nginx config from template
if [ -f /etc/nginx/conf.d/default.conf.template ]; then
  echo "Rendering /etc/nginx/conf.d/default.conf from template with DC_API_URL=$DC_API_URL AUTH_DISABLED=$AUTH_DISABLED"
  # Ensure envsubst substitutes both DC_API_URL and AUTH_DISABLED
  envsubst '${DC_API_URL} ${AUTH_DISABLED}' < /etc/nginx/conf.d/default.conf.template > /etc/nginx/conf.d/default.conf
fi

# Execute the original command (nginx -g 'daemon off;')
exec "$@"
