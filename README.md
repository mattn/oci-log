# oci-log

Send lines to Oracle Cloud Infrastructure Logging Service

## Usage

WARNING: `OCI_LOG_ID` is secret token. Do not share in anywhere.

```
Usage of oci-log.exe:
  -asjson
    	Parse as JSON
  -logid string
    	OCI Log ID ($OCI_LOG_ID)
  -max int
    	Buffer size (default 100)
  -source string
    	OCI Log Source ($OCI_LOG_SOURCE)
  -subject string
    	OCI Log Subject ($OCI_LOG_SUBJECT)
  -tee
    	Tee output
  -type string
    	OCI Log Type ($OCI_LOG_TYPE)
```

```
$ tail -f /var/log/nginx/access.log | oci-log
```

## Installation

```
go install github.com/mattn/oci-log@latest
```

## License

MIT

## Author

Yasuhiro Matsumoto
