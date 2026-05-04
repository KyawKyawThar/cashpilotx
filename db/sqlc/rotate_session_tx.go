package db

import "context"

// RotateSessionTx atomically creates a new session and enforces the max-5
// active-sessions-per-user cap (oldest deleted first). Used on every login
// so that tokens issued by old devices are evicted automatically.

type RotateSessionTxParams struct {
	Session CreateSessionParams
}

type RotateSessionTxResult struct {
	Session Session
}

func (s *SQLStore) RotateSessionTx(ctx context.Context, arg RotateSessionTxParams) (RotateSessionTxResult, error) {
	var result RotateSessionTxResult

	err := s.execTx(ctx, func(q *Queries) error {
		var err error

		result.Session, err = q.CreateSession(ctx, arg.Session)
		if err != nil {
			return err
		}

		const maxSessions = 5
		sessions, err := q.ListUserSessions(ctx, arg.Session.UserID)
		if err != nil {
			return err
		}
		// ListUserSessions returns oldest-first; evict the head when over cap
		for i := 0; i < len(sessions)-maxSessions; i++ {
			if _, err := q.DeleteSessionByID(ctx, sessions[i].ID); err != nil {
				return err
			}
		}

		return nil
	})

	return result, err
}
