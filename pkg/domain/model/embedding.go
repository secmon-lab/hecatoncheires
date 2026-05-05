package model

// EmbeddingDimension is the dimension of the embedding vector produced by the
// configured EmbedClient. The default model (Gemini text-embedding-004) emits
// 768-dimensional vectors. The constant is preserved for the upcoming
// similarity-search redesign that will reintroduce vector-backed features.
const EmbeddingDimension = 768
