# Quicsec
Security wrapper for QUIC protocol.

This project uses [quic-go](https://github.com/lucas-clemente/quic-go) as QUIC implementation.

# QUICk start

Export environment variables:

```
export CERT_FILE=/foo/bar/certs/server.pem
export KEY_FILE=/foo/bar/certs/server.key
```

Run the server app:

```
cd quicsec/examples
go run main.go -www ./www -bind localhost:4433 -v
```

# AuthN by SpiffeID
The spiffe ID from client must be authorized in the configuration to a request be accepted by the server.
The authorized URI should be added to the *AuthIDs* constant in [IdentityManager.go](identity/IdentityManager.go#L17).

ex:
```
AuthIDs = []string{"spiffe://example.com/foo/bar",
					"spiffe://other.com/foo/bar",
					"spiffe://another.com/foo/bar",
					"spiffe://anotherdomain.foo.bar/foo/bar"}
```
It will be part of the ConfManager component in the near future.
# Operations Manager configuration

The configurations are availables in the file [OperationsManager.go](operations/OperationsManager.go#L16). The Operations Manager has four configurables subsystem:
1. Starts the logger in verbose mode
2. Flag to enable dump of pre shared secret and the path file
3. Flag to enable qlog and the path directory
4. Flag to enable tracing metrics using prometheus. Exporting the TLS error
    and some counters (connection duration; transferred bytes
    recv/sent; packets recv/sent; handshake successful; and others)

Also, need to configure `QUICSEC_PROMETHEUS_BIND` in order to export prometheus metrics:
```
export QUICSEC_PROMETHEUS_BIND="192.168.56.101:8080"
```