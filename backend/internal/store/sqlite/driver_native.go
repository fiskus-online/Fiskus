//go:build sqlite && !js

package sqlite

import _ "modernc.org/sqlite" // ovládač „sqlite“ pre database/sql
