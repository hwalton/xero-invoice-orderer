

export $(grep -v '^#' .env | xargs)

# non-blocking, prints URL for manual copy if open fails
STATE=random123
SCOPE='offline_access%20accounting.settings%20accounting.transactions%20accounting.contacts'
URL="https://login.xero.com/identity/connect/authorize?response_type=code&client_id=${XERO_CLIENT_ID}&redirect_uri=${REDIRECT}&scope=${SCOPE}&state=${STATE}"

echo "Open this URL in your browser (or it will try to open it for you):"
echo "$URL"

# try a few non-blocking openers
xdg-open "$URL" >/dev/null 2>&1 & disown || true
python3 -m webbrowser "$URL" >/dev/null 2>&1 & disown || true
gio open "$URL" >/dev/null 2>&1 & disown || true

echo "If nothing opened, copy the URL above into your browser."
echo "Run the callback server (see capture_xero_code.py) to capture the code on http://localhost:8080/xero/callback"