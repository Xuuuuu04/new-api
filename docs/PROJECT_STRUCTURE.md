# Project Structure

Updated: 2026-02-07

## Top-level Layout
```text
.
- .dockerignore
- .env.example
- .gitattributes
- .github
- .gitignore
- Dockerfile
- LICENSE
- README.md
- README.zh.md
- README_EN.md
- VERSION
- bin
- common
- constant
- controller
- docker-compose.yml
- docs
- dto
- frontend
- go.mod
- go.sum
- i18n
- logger
- logs
- main.go
- makefile
- middleware
- model
- new-api.service
- oauth
- one-api.db
- pkg
- relay
- router
- service
- setting
- src
- tools
- types
```

## Conventions
- Keep executable/business code under src/ as the long-term target.
- Keep docs under docs/ (or doc/ for Cangjie projects).
- Keep local runtime artifacts and secrets out of version control.
