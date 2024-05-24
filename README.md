# Harmon Server

Backend service for [harmon-react](https://github.com/danerieber/harmon-react).

# Endpoints

- `/register` HTTP endpoint to generate login token
- `/login` HTTP endpoint to exchange login token for session token
- `/ws` WebSocket endpoint to send/receive JSON data for actions performed by users (authenticated with session token)

# Requirements

Nix users can just use `nix develop` or use [nix-direnv](https://github.com/nix-community/nix-direnv) and `direnv allow .` to automatically load the requirements when you enter the folder.

Otherwise, please manually install go 1.21.9.

# Get Started

To start the server, simply run

```sh
go run *.go
```

# Deployment

See [harmon-deploy](https://github.com/danerieber/harmon-deploy) for examples.
