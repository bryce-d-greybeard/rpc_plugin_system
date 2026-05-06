.PHONY: all build daemon ctl echo failure clean test coverage

all build daemon ctl echo failure clean test coverage:
	$(MAKE) -C src $@
