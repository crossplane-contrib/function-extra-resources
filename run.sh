#!/bin/bash
# It seems extra-resources must be in only one file.
# Supplying the argument a second time excludes any input from the 
# first --extra-resources argument
crossplane beta render \
  --extra-resources example/extraResources.yaml \
  --include-context \
  example/xr.yaml example/composition.yaml example/functions.yaml
