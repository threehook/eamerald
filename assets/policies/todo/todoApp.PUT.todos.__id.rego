package todoApp.PUT.todos.__id

# Completing a todo is allowed for users with the can_write permission on it, and for members of the admin and evil_genius groups.

import data.todoApp.common.check
import data.todoApp.common.is_member_of
import input.resource
import input.user

default allowed := false

allowed if check(user, "can_write", resource.object_id)

allowed if {
	some group in {"admin", "evil_genius"}
	is_member_of(user, group)
}
