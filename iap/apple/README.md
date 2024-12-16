# Local Apple Receipt Verification in Go

This document explains how to locally validate an Apple In-App Purchase (IAP) receipt in Go using the [fullsailor/pkcs7](https://github.com/fullsailor/pkcs7) package and the standard Go `crypto/x509` library. This approach is necessary as Apple’s `verifyReceipt` API is deprecated.

**Disclaimer**: Local receipt validation is complex and potentially fragile. The following outlines our approach.

---

## How This Code Works

1. **Base64 Decoding**  
   We assume the Apple receipt is provided as a base64 string. We first decode it into raw bytes.

2. **Parse PKCS#7**  
   Call `pkcs7.Parse(raw)` to parse the DER-encoded PKCS#7 structure into a `*pkcs7.PKCS7` object.

3. **Verify PKCS#7 Signatures**  
   - `p7.Verify()` checks that the content digest matches the signers’ digests.  
   - **Note**: This does _not_ perform certificate chain verification. It only confirms that the PKCS#7 signature is mathematically valid.

4. **Chain Verification**  
   - We manually verify the certificate chain using `crypto/x509`.  
   - Build an `x509.CertPool` containing Apple’s Root CA (and potentially other Apple intermediate certs).  
   - The PKCS#7 object’s `p7.Certificates` usually contains the leaf certificate plus one or two Apple intermediate certs.  
   - We call:

     ```go
     leaf.Verify(x509.VerifyOptions{
         Roots:         rootPool,        // Apple Root CA
         Intermediates: intermediatePool // Apple intermediate cert(s)
     })
     ```

   to confirm a valid chain to the Apple Root CA.

5. **ASN.1 Parsing**  
   - The `p7.Content` bytes typically represent a `SET OF` “receipt attributes.”  
   - We unmarshal them into `receiptAttribute` structs, which follow Apple’s ASN.1 definition:

     ```asn1
     ReceiptAttribute ::= SEQUENCE {
         type    INTEGER,
         version INTEGER,
         value   OCTET STRING
     }
     ```

6. **Field Checks**  
   - Each attribute has a numeric `type`, a `version` integer, and a `value` (an OCTET STRING).
   - Apple’s official docs specify these important attribute types:
     - **Type 2** → Bundle ID  
     - **Type 3** → App Version  
     - **Type 5** → Device Hash (SHA-1)  
     - **Type 12** → Receipt Creation Date (optional advanced check)  
     - **Type 17** → In-App Purchase info, etc.
   - Compare them against your expected values. If the bundle ID, version, device hash, etc. mismatch, the receipt is invalid.

---

## Apple Root CA

You need to verify the PKCS#7 receipt against Apple’s Root CA. Apple publishes their certificates at [Apple PKI](https://www.apple.com/certificateauthority/). Choose the **Apple Inc. Root Certificate** (or the certificate chain relevant for your receipt). Store it in PEM format, for example:

```go
const AppleRootCAPEM = `-----BEGIN CERTIFICATE-----
MIIFujCCA6KgAwIBAgIQVZAGffJDUjSYFHyw1lwhdjANBgkqhkiG9w0BAQsFADBg
...
-----END CERTIFICATE-----`
