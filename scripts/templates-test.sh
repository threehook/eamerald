#!/usr/bin/env bash

templates=("templates/acmecorp.json" "templates/api-auth.json" "templates/api-gateway.json" "templates/citadel.json" "templates/gdrive.json" "templates/github.json" "templates/multi-tenant.json" "templates/peoplefinder.json" "templates/simple-rbac.json" "templates/slack.json" "templates/todo.json")

tmrld="./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld"

eval "$tmrld version"

for tmpl in ${templates[@]}; do
  echo $tmpl
  # cat $tmpl | jq .

  args="directory delete manifest --force --plaintext"
  ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args

  manifest=$(cat $tmpl | jq -r '.assets.manifest')
  echo $manifest
  args="directory set manifest $PWD/templates/$manifest --plaintext"
  echo $args
  ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args

  idp_data=$(cat $tmpl | jq -r '.assets.idp_data[0]')
  idp_data_dir=$(dirname "$idp_data" )
  echo $idp_data_dir
  args="directory import --directory $PWD/templates/$idp_data_dir --plaintext"
  echo $args
  ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args

  domain_data=$(cat $tmpl | jq -r '.assets.domain_data[0]')
  domain_data_dir=$(dirname "$domain_data" )
  echo $domain_data_dir
  if [[ -z "$domain_data" ]]; then
    echo "NO DOMAIN DATA"
  else
    args="directory import --directory $PWD/templates/$domain_data_dir --plaintext"
    echo $args
    ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args
  fi

  assertion=$(cat $tmpl | jq -r '.assets.assertions[0]')
  echo $assertion
  if [[ -z "$assertion" ]]; then
    echo "NO ASSERTIONS"
  else
    args="directory test exec $PWD/templates/$assertion --summary --plaintext"
    echo $args
    ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args
  fi

  decisions=$(cat $tmpl | jq -r '.assets.assertions[1]')
  echo $decisions
  if [[ -z "$decisions" ]]; then
    echo "NO DECISIONS"
  else
    args="authorizer test exec $PWD/templates/$decisions --summary --plaintext --host localhost:9292"
    echo $args
    ./dist/mrld_$(go env GOOS)_$(go env GOARCH)/mrld $args
  fi
done
