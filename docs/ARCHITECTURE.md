# Architecture

## Components

| Component | Implementation | Responsibility |
| --- | --- | --- |
| Web | React 19 / Vite 6 | Discovery, natural-language requests, authentication, collections |
| API | Go standard `net/http` | Validation, identity, TMDB access, recommendation orchestration |
| Database | PostgreSQL | Users, revocable account sessions, personal activities |
| Catalog | TMDB HTTP API | Search, discovery, details, artwork, credits, keywords |
| Local AI | Ollama | Free-form search planning and evidence evaluation |

There is one backend application. Vite proxies `/api` in development; the Go server can serve the built frontend in a single-port setup.

## Data ownership

- TMDB movie data is fetched on demand and cached in backend memory for up to 10 minutes / 256 responses.
- `users` stores account profiles and bcrypt password hashes.
- `auth_sessions` stores token hashes and expirations; raw tokens live in HttpOnly cookies.
- `activities` stores watchlist/watched/liked/rating state by account or guest identity.
- New accounts are empty. Registration and login never import guest activities automatically.

The embedded SQL migration runs at API startup. It preserves existing activities while removing the legacy local movie-table dependency. Back up data before operating an older database.

## HTTP API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Database health and configured catalog/model names |
| GET | `/api/movies` | Catalog discovery or title search |
| GET | `/api/movies/{id}` | Movie details |
| GET | `/api/genres` | Supported TMDB genre vocabulary |
| GET | `/api/library` | Current account/guest collection |
| PUT | `/api/library/{id}` | Explicitly save personal activity |
| POST | `/api/recommendations` | Recommend from text, genres, or a reference movie |
| POST | `/api/auth/register` | Create an account and session |
| POST | `/api/auth/login` | Authenticate and rotate this browser's session |
| GET | `/api/auth/me` | Read the authenticated profile |
| POST | `/api/auth/logout` | Revoke the current session |

Clients do not select account IDs for collection access. The API resolves identity from a server-validated cookie. Expired sessions return 401 rather than silently selecting a guest collection.

## Recommendation boundaries

The model can interpret arbitrary subjects; TMDB's genre dictionary only maps API IDs. Candidate retrieval remains bounded. Final explanations reference real source snippets, but factual source text alone is not proof of perfect semantic relevance.

The local model is an external runtime dependency and is not included in the repository or Docker image. Unit/HTTP tests mock TMDB and Ollama so CI does not require real tokens or model downloads.

## Delivery

GitHub Actions defines Go race tests with an ephemeral PostgreSQL database, vet, and frontend build/proxy checks. Repository publication and a green remote CI run must be verified separately from local test results.
