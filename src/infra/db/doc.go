// Package db provides database connection and transaction management.
//
// This package is responsible for:
//   - PostgreSQL connection pool initialization
//   - Connection health checks
//   - Transaction management helpers
//   - Query timing and logging (development)
//
// Example usage:
//
//	db, err := db.New(ctx, cfg.Database, log)
//	if err != nil {
//	    return err
//	}
//	defer db.Close()
//
// TODO: Implement migrations support
// TODO: Add query builder integration if needed
package db

