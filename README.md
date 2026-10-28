# aimovieai

**Describe the story you want. Discover your next film.**

A Turkish-first movie discovery app with natural-language recommendations, a live TMDB catalog, and personal film collections. Built with **Go, React, PostgreSQL, and a local Ollama model**.

## What you can do

- **Discover films:** search by title, browse genres, sort, paginate, and open detailed film cards.
- **Describe a story:** ask for a zombie film, a story about perseverance, or a particular setting. Requests are not limited to a fixed topic vocabulary.
- **See grounded recommendations:** suggested films are resolved through TMDB; explanations use actual movie metadata.
- **Keep your collection:** register, sign in, save films, mark them watched, like them, and rate them.
- **Browse as a guest:** new accounts start with an empty collection; guest films are not automatically imported.

## How it works

```mermaid
flowchart LR
    UI[React / Vite] --> API[Go HTTP API]
    API --> DB[(PostgreSQL)]
    API --> TMDB[TMDB: catalog and movie facts]
    API --> LLM[Ollama: search planning and evaluation]
```

1. The local model turns a free-form request into a search meaning, topic keywords, and optional movie-title hints.
2. The API resolves candidates against TMDB. A missing exact keyword does not end the search.
3. Up to eight candidates are evaluated using real English movie evidence while Turkish display metadata is preserved.
4. The model selects up to four strong matches and evidence indexes. The backend rejects unknown movie IDs and invalid evidence references.

Genre/reference-only requests can use content similarity without an LLM. The catalog is fetched on demand, not downloaded into a local movie database.

## Run locally

### Requirements

- Go 1.22+
- Node.js 20.19+ or 22.12+ and npm
- Docker with Compose
- A TMDB **API Read Access Token**
- Ollama with `qwen3:4b` for natural-language recommendations

From the repository root:

```sh
cp .env.example .env
# Set TMDB_TOKEN in .env; keep this file private.
make install

# Start Ollama if it is not already running, then download the model:
ollama pull qwen3:4b

make dev
```

If `.env` already exists, preserve it instead of copying over it.

| Service | Address |
| --- | --- |
| Web app | http://127.0.0.1:5173 |
| API health | http://127.0.0.1:8080/api/health |
| Local PostgreSQL | 127.0.0.1:5434 |

`make dev` starts PostgreSQL, builds the API, and runs Vite. Use Ctrl+C to stop the application, and `make stop` to stop Compose services. Database contents are kept in a named volume; do not remove that volume if you want to keep accounts and collections.

### Configuration

Copy the documented placeholders from [.env.example](.env.example). TMDB credentials stay on the backend and are never included in the frontend bundle.

| Variable | Purpose |
| --- | --- |
| `TMDB_TOKEN` | TMDB API Read Access Token, sent as a Bearer credential |
| `DATABASE_URL` | PostgreSQL connection string |
| `OLLAMA_URL` | Local Ollama API address |
| `OLLAMA_MODEL` | Model name installed in Ollama; default `qwen3:4b` |
| `ADDR` | Backend listening address |
| `FRONTEND_DIST` | Built frontend directory for single-port serving |

The health response checks PostgreSQL. `tmdb_configured` indicates a token exists; it does not verify the token or model availability.

## Verify the project

```sh
make test
cd frontend
../scripts/with-node.sh npm run build
../scripts/with-node.sh npm run test:proxy
```

PostgreSQL integration tests require a **separate test database**:

```sh
cd backend
TEST_DATABASE_URL='postgres://user:password@127.0.0.1:5432/aimovieai_test?sslmode=disable'   go test -race -v ./...
```

The tests create and remove isolated schemas. Never point them at the application database. TMDB/Ollama behavior is tested with fake HTTP transports; passing tests do not prove live recommendation quality.

The [CI workflow](.github/workflows/ci.yml) defines PostgreSQL-backed Go tests, vet, frontend build, and proxy regression checks. Its remote run status is available after the repository is pushed to GitHub.

## Build and deployment

For a single local HTTP port:

```sh
make build
make db
make api
```

Open http://127.0.0.1:8080. For the full Docker profile:

```sh
docker compose --profile full up --build -d
```

The container accesses host Ollama through `host.docker.internal`; host-only loopback binding may prevent access. Configure this deliberately for your environment.

This is a local-first application. An Internet-facing deployment still needs appropriate TLS/reverse-proxy configuration and a production database/password setup. Email verification and password reset are not implemented.

## Project structure

```text
backend/       Go API, authentication, TMDB client, recommendation engine, SQL schema
frontend/      React application and Vite configuration
scripts/       Development and Node runtime helpers
.github/       Automated verification workflow
third_party/   Preserved third-party license notices
```

- [Architecture and API](docs/ARCHITECTURE.md)
- [Authentication](docs/AUTH.md)
- [Free-form recommendation design](docs/FREEFORM-RECOMMENDATIONS.md)
- [Proxy/Origin regression](docs/CSRF-PROXY.md)
- [Verification](docs/VERIFICATION.md)
- [Source acknowledgments](docs/REVIEW.md)

## Practical limits

Recommendations use a bounded candidate pool and depend on available TMDB metadata and model interpretation. Local inference can take around a minute on modest hardware. The system does not claim to search the complete catalog or satisfy every story constraint perfectly. Model scores are not calibrated probabilities.

Passwords are hashed with bcrypt. Account sessions use HttpOnly cookies with server-side revocation. Development credentials in `.env.example` are local placeholders, not production credentials. Keep `.env`, database backups, cookie jars, and local runtime data outside Git.

## Attribution

This product uses the TMDB API but is not endorsed or certified by TMDB. Film metadata and artwork come from TMDB. Third-party license notices are preserved in [third_party/](third_party/).
