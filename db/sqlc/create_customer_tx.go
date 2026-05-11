package db

import (
	"context"
	"fmt"
)

// CreateCustomerTxParams is the input for a standalone customer creation.
type CreateCustomerTxParams struct {
	Customer CreateCustomerParams
}

// CreateCustomerTxResult is the output for a standalone customer creation.
type CreateCustomerTxResult struct {
	Customer Customer
}

// CreateCustomerTx atomically increments the user's customer_sequence and
// inserts the customer with a formatted "C-NNNN" display number.
func (s *SQLStore) CreateCustomerTx(ctx context.Context, arg CreateCustomerTxParams) (CreateCustomerTxResult, error) {
	var result CreateCustomerTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		seq, err := q.NextCustomerSequence(ctx, arg.Customer.UserID)
		if err != nil {
			return err
		}

		num := fmt.Sprintf("C-%04d", seq)
		arg.Customer.CustomerNumber = &num

		result.Customer, err = q.CreateCustomer(ctx, arg.Customer)
		return err
	})

	return result, err
}
