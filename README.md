# Conversor de mídia

Aplicação web pequena para converter imagens e vídeos usando [FFmpeg](https://ffmpeg.org/). A interface permite escolher o formato de saída, consultar informações sobre os formatos na Wikipédia em português e baixar o arquivo convertido.

**Online em [Link](https://converter.henzmiguel.dev:18443/)**.

## Requisitos

- Go 1.27 ou superior
- FFmpeg disponível no `PATH`
- Acesso à internet para carregar Tailwind/daisyUI e consultar a Wikipédia

## Executar localmente

```bash
git clone <url-do-repositorio>
cd converter
go mod download
go run .
```

Abra <http://localhost:8082> no navegador. Para usar outra porta:

```bash
PORT=9090 go run .
```

Para validar ou gerar o executável:

```bash
go test ./...
go build -o converter .
./converter
```

## Executar com Docker

A imagem já instala FFmpeg e executa a aplicação com um usuário sem privilégios:

```bash
docker build -t converter .
docker run --rm -p 8082:8082 converter
```

Para preservar os arquivos convertidos entre reinicializações, monte o diretório `output`:

```bash
docker run --rm -p 8082:8082 \
	-v "$(pwd)/output:/app/output" \
	converter
```

## Funcionalidades

- Conversão de imagens: JPEG, PNG, WebP, GIF, BMP e TIFF.
- Conversão de vídeos: MP4, AVI, MOV, MKV, WebM e FLV.
- Limite de upload de 500 MB.
- Tempo máximo de 10 minutos por conversão.
- Arquivos convertidos removidos automaticamente após 15 minutos.
- Informações de formato carregadas da Wikipédia e armazenadas em cache durante a execução.

## Rotas HTTP

| Método | Rota | Função |
| --- | --- | --- |
| `GET` | `/` | Exibe a interface. |
| `POST` | `/convert` | Recebe o arquivo e inicia a conversão. |
| `GET` | `/download/<arquivo>` | Baixa um arquivo convertido. |
| `GET` | `/formatinfo?format=png` | Retorna informações do formato em JSON. |
| `GET` | `/static/<arquivo>` | Serve os arquivos estáticos embutidos. |

## Estrutura

```text
.
├── main.go                    # Inicialização do servidor e registro das rotas
├── internal/
│   ├── handlers.go             # Handlers HTTP e arquivos embutidos
│   ├── wikipedia.go            # Consulta e cache da Wikipédia
│   ├── cleanFiles.go           # Limpeza periódica de arquivos antigos
│   ├── utils.go                # Funções auxiliares
│   ├── static/app.js           # Comportamento da interface
│   └── templates/index.html    # Página HTML
├── uploads/                    # Arquivos temporários enviados
└── output/                     # Arquivos convertidos
```

`templates/index.html` e `static/app.js` são embutidos no executável com `//go:embed`. Por isso, esses diretórios devem permanecer dentro de `internal`, relativos a `internal/handlers.go`.

## Observações

- O processo precisa conseguir criar e remover arquivos em `uploads/` e `output/`.
- O FFmpeg é verificado ao iniciar, mas uma conversão falhará se ele não estiver instalado.
- Tailwind e daisyUI são carregados por CDN no navegador; sem internet, a conversão continua disponível, mas a aparência da interface pode ser afetada.