# Single Pass Private Information Retrieval

This repository is a fork of the SinglePass PIR project. It includes the core PIR implementation plus two important additions:

- `main` branch: a benchmark harness in `cmd/singlepass_demo_node/main.go`, which measures the time of a single PIR request across several database dimension configurations and reports the data payload received.
- `archi` branch: an IPC interface for remote access to the PIR library, implemented using an RPC server and client in `rpc/`, `cmd/rpc_server`, and `cmd/rpc_client`.

- `cmd/singlepass_demo_node/` - benchmark driver for one-request SinglePass timing
- `cmd/rpc_server/` - RPC server exposing the PIR driver over the network
- `cmd/rpc_client/` - sample RPC client that connects to one or two remote PIR servers

## Benchmark in `main` branch

The benchmark is implemented in `cmd/singlepass_demo_node/main.go`.
It builds a PIR database, derives SinglePass parameters, performs one query per trial, and prints:

- `mean_online_time_seconds`
- `server_to_client_online_bytes_per_query`

### Run the benchmark

From the repository root:

```bash
go run ./cmd/singlepass_demo_node
```

You can override the database size and row length:

```bash
go run ./cmd/singlepass_demo_node -numRows=100000 -rowLen=32
```

The benchmark currently includes multiple predefined dimension configurations and reports timings for each one.

## Notes

- The `cmd/singlepass_demo_node` benchmark is intended to measure single-request performance for different PIR dimension settings.
- The IPC server/client are intended to demonstrate remote access to the PIR library and to support architecture experiments.
