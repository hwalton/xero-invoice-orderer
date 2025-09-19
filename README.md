# Download Make. (Makefile)

# Development:

## 1. Watch CSS changes (from root):
```
make watch-css
```

## 2. Install AIR (on restart):
```
export PATH=$HOME/go/bin:/usr/local/go/bin:$PATH
export PATH="$HOME/go/bin:$PATH"
source ~/.bashrc
```

## 3. Run AIR:
```
cd src/
air
```


# Build for production:

```
make build-css
docker build -f Dockerfile -t flashcards-app .
```
