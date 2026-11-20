// Package e2e holds the end-to-end tests: the bot, built with the e2e tag and run as it's deployed, against a
// throwaway Synapse and PostgreSQL, a scripted model and a stand-in for the network. Test users are real Matrix
// clients. The tests build only with the e2e tag:
//
//	scripts/install-synapse.sh
//	go test -tags e2e ./e2e/...
package e2e
