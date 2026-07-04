package goast

# Ban firestore:"..." struct tags. Domain models under pkg/domain/model are the
# Firestore wire format and are persisted directly via Set(ctx, x) / DataTo(&x);
# an explicit firestore tag encodes a separate wire schema that silently drifts
# from the model (the Repository write-contract bug class). Anchoring on Field
# means a forbidding *comment* mentioning firestore is never matched — only a
# real struct-field Tag is.
fail contains res if {
	input.Kind == "Field"
	input.Node.Tag != null
	contains(input.Node.Tag.Value, "firestore:")

	res := {
		"msg": "do not add firestore:\"...\" struct tags; persist *model.X directly (Repository write contract)",
		"pos": input.Node.Tag.ValuePos,
		"sev": "ERROR",
	}
}
