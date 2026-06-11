package conteudo

import "github.com/tmc/langchaingo/textsplitter"

const (
	// chunkSize é o tamanho máximo de um chunk em caracteres.
	chunkSize = 1500
	// chunkOverlap é a sobreposição entre chunks consecutivos, preservando
	// contexto nas bordas.
	chunkOverlap = 200
)

// splitText divide o texto em chunks com sobreposição, dividindo
// recursivamente por caracteres.
func splitText(text string) ([]string, error) {
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(chunkSize),
		textsplitter.WithChunkOverlap(chunkOverlap),
	)
	return splitter.SplitText(text)
}
