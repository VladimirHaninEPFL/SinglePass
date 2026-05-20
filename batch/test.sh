#!/bin/bash
#SBATCH --nodes 1
#SBATCH --ntasks 1
#SBATCH --cpus-per-task 15
#SBATCH --time 3:00:00
#SBATCH --partition academic
#SBATCH --mem 80G

cd /home/hanin/SinglePass

go run cmd/singlepass_demo_node/main.go