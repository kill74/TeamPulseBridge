## 2026-05-03 - [Security Fix: Timing Attack in ConstantTimeCompare]
**Vulnerability:** `crypto/subtle.ConstantTimeCompare` returns immediately if the lengths of the two inputs are not equal, leaking the length of the expected token.
**Learning:** `subtle.ConstantTimeCompare` is only safe to use when the two inputs are guaranteed to have the same length, or if the expected length is public knowledge. For secrets like Gitlab and Teams tokens where the length is a secret, this leaks information about the secret.
**Prevention:** To safely compare two strings of unknown lengths in constant time without leaking the length, first compute a cryptographic hash (like SHA256) of both the user-provided token and the actual token, then compare the hashes using `subtle.ConstantTimeCompare`. This guarantees both inputs to `ConstantTimeCompare` have the same length (e.g. 32 bytes) and prevents length leakage.
## 2026-05-17 - [Reflected XSS in API Version Error]
**Vulnerability:** [Reflected XSS vulnerability where user input (X-API-Version header) was written directly to the HTTP error response unescaped]
**Learning:** [Untrusted input must never be reflected in HTTP responses without proper sanitization/escaping, even in simple error messages or headers]
**Prevention:** [Always use `html.EscapeString()` or appropriate contextual escaping when including user-provided data in HTTP responses]
