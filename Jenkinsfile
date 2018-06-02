pipeline {
  agent none

  environment {
    CONTAINER_TAG = 'latest'
    ENVIRONMENT = 'ops'
  }

  stages {
    stage('All') {
      agent {
        docker {
          image 'docker.io/controlplane/gcloud-sdk:latest'
          args '-v /var/run/docker.sock:/var/run/docker.sock ' +
              '-v go-build:/root/.cache/go-build ' +
              '--user=root ' +
              '--cap-drop=ALL ' +
              '--cap-add=DAC_OVERRIDE'
        }
      }

      options {
        timeout(time: 25, unit: 'MINUTES')
        retry(1)
        timestamps()
      }

      environment {
        DOCKER_REGISTRY_CREDENTIALS = credentials("${ENVIRONMENT}_docker_credentials")
        SSH_CREDENTIALS = credentials("dev-digitalocean_ssh_credentials")
        K8S_MASTER_HOST = "ipfs-kube-1.ctlplane.io"
      }

      steps {
        ansiColor('xterm') {
          sh """#!/bin/bash

            set -euxo pipefail

            export DOCKER_HUB_USER='${DOCKER_REGISTRY_CREDENTIALS_USR}'
            export DOCKER_HUB_PASSWORD='${DOCKER_REGISTRY_CREDENTIALS_PSW}'
            export SSH_CREDENTIALS_BASE64='${SSH_CREDENTIALS}'

            export K8S_MASTER_HOST

            make test_acceptance
          """
        }
      }
    }
  }
}
