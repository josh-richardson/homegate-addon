.PHONY: e2e-stg e2e-prd

# End-to-end tests against the live API. Requires Mailtrap sandbox creds and
# an E2E_PASSWORD env var. See tests/integration/README.md.

e2e-stg:
	cd tests/integration/harness && E2E_ENV=staging go run .

e2e-prd:
	cd tests/integration/harness && E2E_ENV=production go run .
