#!/bin/bash

# A script to load all dependencies into already exists cluster.
# If you are using specific Kind Cluster name define $KIND_CLUSTER_NAME.

bold=$(tput bold)
normal=$(tput sgr0)

# enable -e after tput to avoid errors on github
set -e
TESTS_DIR=$(dirname `dirname $0`)
DEPENDENCIES_DIR="$TESTS_DIR/dependencies"

if [ -n "$KIND_CLUSTER_NAME" ]; then
    KIND_LOAD_OPTIONS="--name $KIND_CLUSTER_NAME"
fi

yamls=("cert-manager_1.8.yaml")

function deps() {
    for y in ${yamls[@]}; do
        kubectl apply -f ${DEPENDENCIES_DIR}/$y
    done
    istioctl install --set profile=demo -y
    # cert-manager seems to start slowly
    echo ""
    echo -n "Waiting 30 seconds for everything to initialize properly ..."
    sleep 30
    echo " Done"
}

function deploy() {
    make docker-build
    kind load docker-image $KIND_LOAD_OPTIONS controller:latest
    echo ""
    make deploy
}

function remove() {
    make undeploy
    for y in ${yamls[@]}; do
        kubectl delete -f ${DEPENDENCIES_DIR}/$y
    done
    echo "${bold}Note: this script does not uninstall Istio!${normal}"
}

function create() {
    env_file="${TESTS_DIR}/env"
    . $env_file
    kind create cluster --image $KIND_CUSTOM_IMAGE
}


case "$1" in
    up)
        deps
        deploy
        ;;
    down)
        remove
        ;;
    create)
        create
        ;;
    delete)
        kind delete cluster
        ;;
    deps)
        deps
        ;;
    deploy)
        deploy
        ;;
    *)
        echo "Usage $0 <COMMAND>"
        echo ""
        echo "COMMAND is one of:"
        echo "  - up:     Populate cluster with required dependencies and controller."
        echo "  - down:   Cleanup cluster from all deps and controller."
        echo "  - create: Creates a kind cluster with default name."
        echo "  - delete: Deletes the default cluster."
        echo "  - deps:   Install required dependencies."
        echo "  - deploy: Deploys the controller."
        echo ""
        echo "Only for deploying the controller: if the cluster name is not the default one use KIND_CLUSTER_NAME environment variable to define the cluster name."
        echo ""
        echo "${bold}NOTE: All commands assumes your kubernetes config points to the KIND cluster!${normal}"
        ;;
esac
