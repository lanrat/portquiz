module portquiz/android/gomobile

go 1.27.2

require portquiz v0.0.0

require (
	golang.org/x/mobile v0.0.0-20260908204917-8b95e45f8d3e // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools v0.53.0 // indirect
)

replace portquiz => ../..

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
