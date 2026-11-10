module github.com/NARUBROWN/spine

go 1.25.5

// Missing multiple scalar input.Path binding promised by the CLI; use v0.7.1 or later.
retract v0.7.0

// Missing the explicit input.Header API; use v0.7.2-preview.2 or later for header wrappers.
retract v0.7.1

// Still accepts unwrapped HTTP DTO inputs; use the explicit-wrapper hotfix candidate.
retract v0.7.2-preview.1
