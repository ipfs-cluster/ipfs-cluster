#!/usr/bin/env bash
#
# IPFS Cluster Acceptance Tests
#
## Usage: %SCRIPT_NAME% [options] filename
##
## Options:
##   --docker-user [user]              Docker hub username
##   --docker-password [pass]          Docker hub password
##   --ssh-credentials-base64 [creds]  SSH credentials to access k8s nodes, base64 encoded
##   --k8s-master-host [host]          Hostname or IP of K8S master
##
##   --debug                     More debug
##   -h --help                   Display this message
##

# exit on error or pipe failure
set -eo pipefail
# error on unset variable
set -o nounset
# error on clobber
set -o noclobber

# user defaults
DESCRIPTION="IPFS Cluster Acceptance Tests"
DEBUG=0

K8S_MASTER_HOST="${K8S_MASTER_HOST:-ipfs-kube-1.ctlplane.io}"
SSH_CREDENTIALS_BASE64="${SSH_CREDENTIALS_BASE64:-}"
DOCKER_REGISTRY_CREDENTIALS_PSW="${DOCKER_REGISTRY_CREDENTIALS_PSW:-}"
DOCKER_REGISTRY_CREDENTIALS_USR="${DOCKER_REGISTRY_CREDENTIALS_USR:-}"

CONTAINER_TAG='latest'
TMP_SSH_KEYFILE=$(mktemp)

# resolved directory and self
declare -r DIR=$(cd "$(dirname "$0")" && pwd)
declare -r THIS_SCRIPT="${DIR}/$(basename "$0")"

# required defaults
declare -a ARGUMENTS
EXPECTED_NUM_ARGUMENTS=0
ARGUMENTS=()

main() {
  handle_arguments "$@"

  docker_hub_login

  docker_build

  docker_build_test_image

  push_docker_test_image

  run_k8s_tests

  success 'Acceptance tests passed'
}

docker_hub_login() {
  docker login \
    --username "${DOCKER_HUB_USER}" \
    --password "${DOCKER_HUB_PASSWORD}"
}

docker_build() {
  make docker
}

docker_build_test_image() {
  make docker-build-test-image \
    CONTAINER_TAG="${CONTAINER_TAG}"
}

push_docker_test_image() {
  make docker-push-test-image \
    CONTAINER_TAG="${CONTAINER_TAG}"
}

run_k8s_tests() {
  setup_temp_dir

  fetch_kubeconfig_from_k8s_master

  get_tests_from_docker_image

  k8s_init

  run_yaml_tests
}

fetch_kubeconfig_from_k8s_master() {
  trap "rm -rf '${TMP_SSH_KEYFILE}' '${TEMP_DIR}' /tmp/admin.conf" EXIT

  echo "${SSH_CREDENTIALS_BASE64}" \
    | base64 -d >>"${TMP_SSH_KEYFILE}"
  chmod 600 "${TMP_SSH_KEYFILE}"

  mkdir -p ~/.ssh/ || true
  ssh-keyscan ${K8S_MASTER_HOST} >>~/.ssh/known_hosts
  scp -i "${TMP_SSH_KEYFILE}" \
    root@${K8S_MASTER_HOST}:/etc/kubernetes/admin.conf /tmp/

  K8S_MASTER_IP=$(getent hosts "${K8S_MASTER_HOST}" | awk '{print $1}')

  export KUBECONFIG="/tmp/admin.conf"
  sed -E "s,(server: https://)[^:]*(:6443.*),\\1${K8S_MASTER_IP}\\2,g" -i "${KUBECONFIG}"

  kubectl get nodes
}

setup_temp_dir() {
  TEMP_DIR=$(mktemp -d)
  cd "${TEMP_DIR}"
}

get_tests_from_docker_image() {
  local CID_FILE CID

  CID_FILE=$(mktemp --dry-run)
  echo "${CID_FILE}"

  docker pull controlplane/kubernetes-ipfs:latest
  docker run \
    -d \
    --cidfile=${CID_FILE} \
    controlplane/kubernetes-ipfs:latest \
    sleep 999

  CID=$(cat "${CID_FILE}")
  docker cp \
    ${CID}:/code/ \
    .

  docker kill ${CID}

  cd code

  ./kubernetes-ipfs --help || true
}

k8s_init() {
  ./init.sh
}

run_yaml_tests() {
  local IS_FAILED=0
  local FAILING_TESTS=()

  # TODO: does not currently test
  # - kubernetes-ipfs/tests/archives/archives-test.yml
  # - add-and-gc-template.yml
  # preprocessing is required
  for YML in $(find tests \
    -type f \
    -name '*.yml' \
    | grep -Ev '(template.yml|archives-test.yml)$'); do

    hr
    echo "Starting test: ${YML}"
    hr

    if ! ./kubernetes-ipfs "${YML}"; then
      warning "Test failed: ${YML}"
      IS_FAILED=1
      FAILING_TESTS+=("${YML}")
    fi

    echo
    hr
    echo "Ending test: ${YML}"
    hr
    printf -- '\n\n'
  done

  if [[ "${IS_FAILED:-}" != 0 ]]; then
    warning "Failing tests: ${FAILING_TESTS[@]}"
    error "Acceptance tests failed"
  fi
}

handle_arguments() {
  [[ $# = 0 && "${EXPECTED_NUM_ARGUMENTS}" -gt 0 ]] && usage

  parse_arguments "$@"
  validate_arguments "$@"
}

parse_arguments() {
  while [ $# -gt 0 ]; do
    case $1 in
      -h | --help) usage ;;
      --debug)
        DEBUG=1
        set -xe
        ;;
      --docker-user)
        shift
        not_empty_or_usage "${1:-}"
        DOCKER_HUB_USER="${1}"
        ;;
      --docker-password)
        shift
        not_empty_or_usage "${1:-}"
        DOCKER_HUB_PASSWORD="${1}"
        ;;
      --ssh-credentials-base64)
        shift
        not_empty_or_usage "${1:-}"
        SSH_CREDENTIALS_BASE64="${1}"
        ;;
      --k8s-master-host)
        shift
        not_empty_or_usage "${1:-}"
        K8S_MASTER_HOST="${1}"
        ;;
      --)
        shift
        break
        ;;
      -*) usage "${1}: unknown option" ;;
      *) ARGUMENTS+=("$1") ;;
    esac
    shift
  done
}

validate_arguments() {
  check_number_of_expected_arguments

  [[ -n "${DOCKER_HUB_USER:-}" ]] || error "--docker-user required"
  [[ -n "${DOCKER_HUB_PASSWORD:-}" ]] || error "--docker-password required"

  [[ -n "${SSH_CREDENTIALS_BASE64:-}" ]] || error "--ssh-credentials-base64 required"

  [[ -n "${K8S_MASTER_HOST:-}" ]] || error "--k8s-master-host required"
}

# helper functions

usage() {
  [ "$*" ] && echo "${THIS_SCRIPT}: ${COLOUR_RED}$*${COLOUR_RESET}" && echo
  sed -n '/^##/,/^$/s/^## \{0,1\}//p' "${THIS_SCRIPT}" | sed "s/%SCRIPT_NAME%/$(basename "${THIS_SCRIPT}")/g"
  exit 2
} 2>/dev/null

success() {
  [ "${*:-}" ] && RESPONSE="$*" || RESPONSE="Unknown Success"
  printf "%s\n" "$(log_message_prefix)${COLOUR_GREEN}${RESPONSE}${COLOUR_RESET}"
} 1>&2

info() {
  [ "${*:-}" ] && INFO="$*" || INFO="Unknown Info"
  printf "%s\n" "$(log_message_prefix)${COLOUR_WHITE}${INFO}${COLOUR_RESET}"
} 1>&2

warning() {
  [ "${*:-}" ] && ERROR="$*" || ERROR="Unknown Warning"
  printf "%s\n" "$(log_message_prefix)${COLOUR_RED}${ERROR}${COLOUR_RESET}"
} 1>&2

error() {
  [ "${*:-}" ] && ERROR="$*" || ERROR="Unknown Error"
  printf "%s\n" "$(log_message_prefix)${COLOUR_RED}${ERROR}${COLOUR_RESET}"
  exit 3
} 1>&2

error_env_var() {
  error "${1} environment variable required"
}

log_message_prefix() {
  local TIMESTAMP="[$(date +'%Y-%m-%dT%H:%M:%S%z')]"
  local THIS_SCRIPT_SHORT=${THIS_SCRIPT/$DIR/.}
  tput bold 2>/dev/null
  echo -n "${TIMESTAMP} ${THIS_SCRIPT_SHORT}: "
}

is_empty() {
  [[ -z ${1-} ]] && return 0 || return 1
}

not_empty_or_usage() {
  is_empty "${1-}" && usage "Non-empty value required" || return 0
}

check_number_of_expected_arguments() {
  [[ "${EXPECTED_NUM_ARGUMENTS}" != "${#ARGUMENTS[@]}" ]] && {
    ARGUMENTS_STRING="argument"
    [[ "${EXPECTED_NUM_ARGUMENTS}" -gt 1 ]] && ARGUMENTS_STRING="${ARGUMENTS_STRING}"s
    usage "${EXPECTED_NUM_ARGUMENTS} ${ARGUMENTS_STRING} expected, ${#ARGUMENTS[@]} found"
  }
  return 0
}

hr() {
  printf '=%.0s' $(seq $(tput cols))
  echo
}

wait_safe() {
  local PIDS="${1}"
  for JOB in ${PIDS}; do
    wait "${JOB}"
  done
}

export CLICOLOR=1
export TERM="xterm-color"
export COLOUR_BLACK=$(tput setaf 0 :-"" 2>/dev/null)
export COLOUR_RED=$(tput setaf 1 :-"" 2>/dev/null)
export COLOUR_GREEN=$(tput setaf 2 :-"" 2>/dev/null)
export COLOUR_YELLOW=$(tput setaf 3 :-"" 2>/dev/null)
export COLOUR_BLUE=$(tput setaf 4 :-"" 2>/dev/null)
export COLOUR_MAGENTA=$(tput setaf 5 :-"" 2>/dev/null)
export COLOUR_CYAN=$(tput setaf 6 :-"" 2>/dev/null)
export COLOUR_WHITE=$(tput setaf 7 :-"" 2>/dev/null)
export COLOUR_RESET=$(tput sgr0 :-"" 2>/dev/null)

main "$@"
