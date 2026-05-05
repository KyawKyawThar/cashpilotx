package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	password := RandomString(7)

	hashPassword, err := HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hashPassword)

	err = CheckPassword(password, hashPassword)
	require.NoError(t, err)

	wrongPassword := RandomString(6)
	err = CheckPassword(wrongPassword, hashPassword)
	require.EqualError(t, err, bcrypt.ErrMismatchedHashAndPassword.Error())

	hashPassword2, err := HashPassword(password)
	require.NoError(t, err)
	require.NotEmpty(t, hashPassword2)
	require.NotEqual(t, hashPassword2, hashPassword)
}

func TestValidatePassword(t *testing.T) {

	minLength := 8

	testCases := []struct {
		name     string
		password string
		isError  bool
	}{
		{
			name:     "valid password",
			password: RandomString(10),
			isError:  false,
		},
		{
			name:     "exact min length",
			password: RandomString(8),
			isError:  false,
		},
		{
			name:     "too short",
			password: RandomString(6),
			isError:  true,
		},
		{
			name:     "empty password",
			password: "",
			isError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.password, minLength)
			if tc.isError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "password does not meet requirements")
			} else {
				require.NoError(t, err)
			}

		})
	}

}
