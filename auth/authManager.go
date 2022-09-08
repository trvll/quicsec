package auth

import (
	"crypto/x509"
	"fmt"

	"github.com/quicsec/quicsec/identity"
)

func QuicsecVerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {

	if len(rawCerts) != 1 {
		return fmt.Errorf("auth: required exactly one peer certificate")
	}

	cert, err := x509.ParseCertificate(rawCerts[0])

	if err != nil {
		return fmt.Errorf("auth: failed to parse peer certificate")
	}

	for _, uri := range cert.URIs {
		fmt.Println("Validating URI: ", uri)
		rv := identity.VerifyIdentity(uri.String())

		if rv {
			fmt.Println("Authorized!")
			return nil
		} else {
			fmt.Println("Not Authorized!")
		}
	}

	return fmt.Errorf("auth: No valid spiffe ID was found =(")
}
