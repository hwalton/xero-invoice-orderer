export $(grep -v '^#' ../.env | xargs)

curl -sS -X POST "https://identity.xero.com/connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -H "Authorization: Basic $(printf '%s:%s' "$XERO_CLIENT_ID" "$XERO_CLIENT_SECRET" | base64 -w0)" \
  -d "grant_type=authorization_code&code=${XERO_AUTH_CODE}&redirect_uri=${REDIRECT}" | jq .