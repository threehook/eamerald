package peoplefinder.PUT.api.users.__id

# Updating an employee is allowed for admins and for the employee themselves. It is always visible and enabled.

import input.user.properties.roles as user_roles

default allowed := false

default visible := true

default enabled := true

allowed if "admin" in user_roles

allowed if input.user.id == input.resource.id
