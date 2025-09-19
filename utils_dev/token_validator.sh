TOKEN='eyJhbGciOiJIUzI1NiIsImtpZCI6IlZEL0hHa2FtOERGVXllQlEiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2duamZlZ29qcG9tY3RiYXh4ZHRmLnN1cGFiYXNlLmNvL2F1dGgvdjEiLCJzdWIiOiIxM2RmYjFlZi0xNzI3LTQwZjctYTYwYS00ODQxODM4NzgwNGIiLCJhdWQiOiJhdXRoZW50aWNhdGVkIiwiZXhwIjoxNzU4MzEwOTc1LCJpYXQiOjE3NTgzMDczNzUsImVtYWlsIjoiaGFydmV5d2FsdG9uLmh3QGdtYWlsLmNvbSIsInBob25lIjoiIiwiYXBwX21ldGFkYXRhIjp7InByb3ZpZGVyIjoiZW1haWwiLCJwcm92aWRlcnMiOlsiZW1haWwiXX0sInVzZXJfbWV0YWRhdGEiOnsiZW1haWxfdmVyaWZpZWQiOnRydWV9LCJyb2xlIjoiYXV0aGVudGljYXRlZCIsImFhbCI6ImFhbDEiLCJhbXIiOlt7Im1ldGhvZCI6InBhc3N3b3JkIiwidGltZXN0YW1wIjoxNzU4MzA3Mzc1fV0sInNlc3Npb25faWQiOiIyYTIxZGNmMi1iMDc3LTQ4NGItOGFjNy04NjkwMGE1OTlhNmMiLCJpc19hbm9ueW1vdXMiOmZhbHNlfQ.NZPvU0peHRBMiMAl15XMHjWVPz2xh8Piu1G0t3SwsLw'

# header
echo -n "$TOKEN" | awk -F. '{print $1}' | \
  python3 -c "import sys,base64,json;h=sys.stdin.read().strip();h += '='*((4-len(h)%4)%4);print(json.dumps(json.loads(base64.urlsafe_b64decode(h)), indent=2))"

# payload
echo -n "$TOKEN" | awk -F. '{print $2}' | \
  python3 -c "import sys,base64,json;h=sys.stdin.read().strip();h += '='*((4-len(h)%4)%4);print(json.dumps(json.loads(base64.urlsafe_b64decode(h)), indent=2))"