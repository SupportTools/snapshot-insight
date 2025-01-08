# Snapshot Insight

Snapshot Insight is a CLI tool designed to explore Kubernetes etcd snapshots without requiring a full cluster restoration. It allows you to:

- Restore etcd snapshots into a standalone Docker container.
- Start a kube-apiserver connected to the restored etcd snapshot with no authentication.
- Clean up all resources after exploration.

## Features

- **Quick Restoration**: Restore etcd snapshots directly into a container.
- **Standalone Exploration**: Run a kube-apiserver in standalone mode to access cluster resources.
- **Lightweight Cleanup**: Remove all temporary containers and files with a single command.

## Getting Started

### Prerequisites

- [Docker](https://www.docker.com/) installed and running.
- Go 1.18 or later installed.

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/supporttools/snapshot-insight.git
   cd snapshot-insight
   ```

2. Build the project:
   ```bash
   go build -o snapshot-insight ./cmd/snapshot-insight
   ```

3. Run the CLI tool:
   ```bash
   ./snapshot-insight
   ```

## Usage

### Commands

#### Restore
Restores an etcd snapshot into a Docker container.
```bash
./snapshot-insight restore <path-to-snapshot>
```

#### Start
Starts a kube-apiserver connected to the restored etcd container.
```bash
./snapshot-insight start
```

#### Cleanup
Stops and removes the etcd and kube-apiserver containers.
```bash
./snapshot-insight cleanup
```

## Development

### Running Tests

Run unit tests with:
```bash
go test ./...
```

### Scripts

- `run.sh`: Automates restoring an etcd snapshot and starting the kube-apiserver.
- `teardown.sh`: Cleans up all resources created during runtime.

## Contributing

Contributions are welcome! Feel free to submit issues or pull requests.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Author

Matthew Mattox

---

### Example

```bash
# Restore an etcd snapshot
./snapshot-insight restore /path/to/snapshot.db

# Start kube-apiserver
./snapshot-insight start

# Explore resources using kubectl
kubectl --server=http://localhost:6443 get pods

# Clean up resources
./snapshot-insight cleanup
