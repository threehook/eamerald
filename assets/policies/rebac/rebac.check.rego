package rebac.check

# Allows a request when the directory holds the requested relation or permission between the subject and the object.
#
# input.resource carries the object_type, object_id and relation (or permission) to check.
# The subject is the authenticated user. For a subject that is not a user, set input.identity.type to
# IDENTITY_TYPE_MANUAL, input.identity.identity to the subject id and input.resource.subject_type to the subject type.

default allowed := false

default subject_type := "user"

allowed if {
	ds.check({
		"object_type": input.resource.object_type,
		"object_id": input.resource.object_id,
		"relation": input.resource.relation,
		"subject_type": subject_type,
		"subject_id": subject_id,
	})
}

subject_type := input.resource.subject_type if {
	input.identity.type == "IDENTITY_TYPE_MANUAL"
	input.resource.subject_type != ""
}

subject_id := input.identity.identity if {
	input.identity.type == "IDENTITY_TYPE_MANUAL"
} else := input.user.id
