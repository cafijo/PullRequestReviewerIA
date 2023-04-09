#!/bin/sh -l
/main --openai_api_key "$1" --github_token "$2" --github_pr_id $3 --github_user "$4" --github_repo "$5"