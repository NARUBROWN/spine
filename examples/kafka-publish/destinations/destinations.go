package destinations

import "github.com/NARUBROWN/spine/contract/event"

const OrderCreated = "orders.created"

type Catalog struct{}

func (Catalog) PublishDestinations() []string {
	return []string{OrderCreated}
}

var _ eventcontract.DestinationSource = Catalog{}
