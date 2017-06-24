This is a very simple GOLANG web server that hosts the same content via 4 protocols:
* http1.1 (port 8080)
* http1.1 + tls (port 8081)
* http2 (port 8082)
* QUIC (port 8083)

It can be used to show large-scale differences in protocol performance (e. g. quic is a lot faster when >1% packets are lost).

It is not at all suitable for benchmarking because atm no open-source quic implementation is optimised for performance (neither client nor server).

To run this server you will have to put relevant certificates in a folder specified in `getSslFiles()` (todo: accept cert paths as args) and install all dependencies for this app (`go get ./...`).

Two html pages were made to showcase protocol differences from *.nikitin.su. In order to reuse these demo pages feel free to find-and-replace `nikitin.su` and go from there: 
* /benchmark — runs a few demo cases for each protocol
* /ui — runs some pseudo-cache demo of http prefetching capabilities

If you have any questions at all — please open an issue.