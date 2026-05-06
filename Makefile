.PHONY: all build daemon ctl echo failure clean test coverage doc-check

all build daemon ctl echo failure clean coverage:
	$(MAKE) -C src $@

test: doc-check
	$(MAKE) -C src test

doc-check:
	scripts/check-protocol-doc-honesty.sh
