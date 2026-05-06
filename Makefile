.PHONY: all build daemon ctl echo failure clean test

all build daemon ctl echo failure clean test:
	$(MAKE) -C src $@
