package todoApp.DELETE.todos.__id

# Deleting a todo is allowed for users with the can_delete permission on it, and for members of the admin group.

import data.todoApp.common.check
import data.todoApp.common.is_member_of
import input.resource
import input.user

default allowed := false

allowed if check(user, "can_delete", resource.object_id)

allowed if is_member_of(user, "admin")
