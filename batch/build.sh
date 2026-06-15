#!/bin/bash
#SBATCH --nodes 1
#SBATCH --ntasks 1
#SBATCH --cpus-per-task 5
#SBATCH --time 1:00:00
#SBATCH --partition academic
#SBATCH --mem 10G

cd /home/hanin/SinglePass

go build -o pir-server cmd/singlepass_demo_node/server/server.go
go build -o pir-client cmd/singlepass_demo_node/client/client.go