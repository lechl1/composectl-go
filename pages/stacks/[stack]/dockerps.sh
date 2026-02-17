#!/usr/bin/env bash

docker ps --format json | jq -s .