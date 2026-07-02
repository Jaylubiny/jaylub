#!/usr/bin/env bash
kill $(cat server.pid)
echo "Stopped application server process."
