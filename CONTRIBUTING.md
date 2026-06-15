# Contributing to PrismGateway

Contributions are welcome. Please read this before submitting a pull request.

## Before you start

Open an issue first for any non-trivial change (new feature, significant refactor). This avoids wasted effort if the direction doesn't fit the project goals.

For bug fixes and small improvements, a PR is fine without an issue.

## Development setup

```bash
git clone https://github.com/diegomcastronuovo/prism-gateway.git
cd prism-gateway
cp config.example.yaml config.yaml
cp .env.example .env
# fill in at least one provider key in .env
make up
```

## Running tests

```bash
make test
```

Backend tests run without any external services. Integration tests require `DATABASE_URL` (set automatically when using `make up`).

## Submitting a PR

1. Fork the repo and create a feature branch (`git checkout -b feat/my-feature`)
2. Keep commits focused — one logical change per commit
3. Run `make test` before pushing
4. Open a PR against `main` with a clear description of what and why

## Code style

- **Backend (Go)**: standard `gofmt` formatting; run `go vet ./...` before committing
- **Frontend (Next.js)**: Prettier + ESLint (run `npm run lint` in `frontend/`)

## What belongs in community vs enterprise

Community (this repo):
- Core routing logic (semantic, cost, PII, tool routing)
- Budget enforcement and rate limiting
- Semantic caching and circuit breaker
- Admin UI for all of the above

Enterprise (not accepted here):
- DecisionOps workflow engine
- FinOps dashboards (CFO Board, anomaly detection)
- MRM and compliance modules
- Claude Code integration

## License

By contributing you agree that your contributions will be licensed under the [MIT License](LICENSE).
