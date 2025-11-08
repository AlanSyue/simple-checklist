#!/bin/bash

echo "Stopping and removing existing Docker containers..."
docker-compose down

echo "Rebuilding and restarting Docker containers..."
docker-compose up --build -d

echo "Done."
