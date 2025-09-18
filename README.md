# Download Make. (Makefile)

# Install AIR (on restart):
```
export PATH=$HOME/go/bin:/usr/local/go/bin:$PATH
export PATH="$HOME/go/bin:$PATH"
source ~/.bashrc
```

# Development:
```
make watch-css
cd src/
air
```


# Build for production:

```
make build-css
docker build -f Dockerfile -t flashcards-app .
```
