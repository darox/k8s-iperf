.PHONY: all clean

all:
	@make -f Makefile.binary all
	@make -f Makefile.iperf docker-build
	@make -f Makefile.k8s-iperf docker-build

clean:
	@make -f Makefile.binary clean
	@make -f Makefile.iperf docker-clean
	@make -f Makefile.k8s-iperf docker-clean