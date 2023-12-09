module github.com/jotaen/kong-completion

go 1.20

require (
	github.com/alecthomas/kong v0.7.1
	github.com/posener/complete/v2 v2.1.0
	github.com/riywo/loginshell v0.0.0-20200815045211-7d26008be1ab
)

require github.com/posener/script v1.2.0 // indirect

retract (
	v1.0.2 // Published accidentally.
	v1.0.1 // Published accidentally.
	v1.0.0 // Published accidentally.
)
