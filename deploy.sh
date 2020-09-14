#!/usr/bin/env bash

ytt -f ./k8s/ \
| ko apply --context=docker-desktop --local --preserve-import-paths -f -
