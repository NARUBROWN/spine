package componentregistration

type Registration struct {
	role  string
	value any
}

func New(role string, value any) Registration {
	return Registration{role: role, value: value}
}

func Unpack(registration Registration) (role string, value any) {
	return registration.role, registration.value
}
