package dalgo2mysql

// SupportsTransactionalDDL reports that MySQL does NOT support transactional
// DDL. Unlike PostgreSQL, MySQL implicitly commits the current transaction
// before (and after) every DDL statement such as CREATE / DROP / ALTER, so
// DDL cannot be rolled back atomically. Callers must not rely on
// BEGIN/ROLLBACK to undo schema changes.
func (d *Database) SupportsTransactionalDDL() bool { return false }
