package types

import "cosmossdk.io/collections"

// PaymentRecordKey is the prefix to retrieve all PaymentRecord
var PaymentRecordKey = collections.NewPrefix("paymentRecord/value/")
