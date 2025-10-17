#!/usr/bin/env bash

set -euo pipefail

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "Error: script must be run inside a git repository." >&2
  exit 1
fi

BRANCH_DOCSPRING="docspring"
BRANCH_MASTER="master"

# Ensure we have a clean worktree before doing anything destructive.
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: working tree has uncommitted changes. Please commit or stash them first." >&2
  exit 1
fi

git checkout "${BRANCH_DOCSPRING}"
git fetch origin "${BRANCH_MASTER}" >/dev/null 2>&1 || true
git reset --hard "${BRANCH_MASTER}"

# Iterate over all local branches except master and docspring.
while IFS= read -r branch; do
  [[ "${branch}" == "${BRANCH_MASTER}" ]] && continue
  [[ "${branch}" == "${BRANCH_DOCSPRING}" ]] && continue
  [[ "${branch}" == "fix/use_config_map_for_cleanup_timestamp" ]] && continue
  [[ "${branch}" == "docspring-branch-script" ]] && continue
  echo "Merging branch '${branch}' into ${BRANCH_DOCSPRING}..."
  if ! git merge --no-edit "${branch}"; then
    echo "Merge conflict detected for branch '${branch}'."
    echo "Applying known conflict resolutions..."

    # Apply specific patches for known conflicts
    if [[ "${branch}" == "fix/reduce_terraform_drift" ]]; then
      # For fix/reduce_terraform_drift: accept lifecycle ignore_changes blocks
      if git diff --name-only --diff-filter=U | grep -q "terraform/api/k8s/main.tf"; then
        echo "  Accepting lifecycle ignore_changes blocks in terraform/api/k8s/main.tf..."
        git checkout --theirs -- terraform/api/k8s/main.tf
        git add terraform/api/k8s/main.tf
      fi
    fi

    if [[ "${branch}" == "rack-release-override" ]]; then
      # For rack-release-override: resolve private_api conflicts by keeping the line
      for file in terraform/system/aws/main.tf terraform/system/do/main.tf terraform/system/local/main.tf; do
        if git diff --name-only --diff-filter=U | grep -q "${file}"; then
          echo "  Resolving private_api conflict in ${file}..."
          # Remove conflict markers but keep the private_api line
          # This handles conflicts like:
          # <<<<<<< HEAD
          #   private_api                               = var.private_api
          # =======
          # >>>>>>> rack-release-override
          sed -i '' '/^<<<<<<< HEAD$/,/^>>>>>>> rack-release-override$/{
            /^<<<<<<< HEAD$/d
            /^=======$/d
            /^>>>>>>> rack-release-override$/d
          }' "${file}"

          git add "${file}"
        fi
      done
    fi

    # Complete the merge
    git commit --no-edit
    echo "Conflicts resolved. Merge completed."
  fi
done < <(git for-each-ref --format='%(refname:short)' refs/heads/)

echo "All branches merged into ${BRANCH_DOCSPRING}."
