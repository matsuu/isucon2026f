#!/bin/sh

PGPASSWORD=isureal psql -h asia2 -U isureal "$@"
