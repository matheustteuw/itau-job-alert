# itau-job-alert

Monitora vagas de **Tecnologia** do Itaú, do PicPay e do BTG Pactual e manda
um e-mail quando aparece uma vaga nova que bate com seu filtro de
palavras-chave. Cada execução faz UMA checagem (não fica em loop infinito) —
quem controla a frequência é o agendador (EventBridge no Lambda,
cron/systemd se rodar localmente).

## Como funciona

1. Baixa as vagas do Itaú (`carreiras.itau.com.br`, HTML), do PicPay (API
   pública do Oracle Cloud HCM, JSON) e do BTG Pactual (API pública oficial
   do Greenhouse, JSON).
2. Extrai título + link de cada vaga, das três fontes.
3. Filtra pelo título: Itaú e PicPay usam `KEYWORDS`, o BTG usa
   `BTG_KEYWORDS` (mais restrito, já que o board dele cobre a empresa
   inteira). Em qualquer fonte, descarta as que batem com
   `EXCLUDE_KEYWORDS` (veja [Filtro de vagas](#filtro-de-vagas)).
4. Compara com o histórico do que já foi visto (`seen_jobs.json`, local ou no S3).
5. Se tiver vaga nova relevante, manda **um e-mail só** listando todas as
   novas (de qualquer empresa) e atualiza o histórico.

Não faz candidatura automática — só avisa. Dá pra evoluir depois.

## Filtro de vagas

Por padrão só entram vagas cujo título contenha alguma destas palavras:

```
engenheiro, engenharia, java, .net, desenvolvedor
```

A comparação ignora acento e maiúsc/minúsc, então "Júnior", "JUNIOR" e
"junior" batem igual. Pra customizar, defina `KEYWORDS` no `.env` (local) ou
como variável de ambiente da função Lambda, separando por vírgula:

```
KEYWORDS=Engenheiro,Engenharia,Java,.NET,Desenvolvedor,Junior,Pleno,Senior,Backend,Fullstack
```

Isso dá pra ajustar sem recompilar/redeployar código — só mudar a env var no
console do Lambda (ou no `.env` local) e a próxima execução já usa o novo filtro.

Além disso, vaga cujo título bate com `EXCLUDE_KEYWORDS` é descartada mesmo
que bata com `KEYWORDS` — a exclusão tem prioridade. Por padrão isso derruba
vagas afirmativas exclusivas pra pessoas com deficiência (`pcd`,
`deficiencia`), já que tanto o Itaú ("Exclusiva para Pessoas com
deficiência") quanto o PicPay ("Exclusiva PCD") marcam isso no próprio
título da vaga. Customizável do mesmo jeito que `KEYWORDS`:

```
EXCLUDE_KEYWORDS=pcd,deficiencia,estagio
```

### Filtro à parte pro BTG Pactual

O board de vagas do BTG cobre a empresa inteira — trading, risco, vendas,
etc. — não só Tecnologia como o do Itaú, nem majoritariamente tech como o do
PicPay. Por isso usa uma lista separada, `BTG_KEYWORDS`, mais restrita que
`KEYWORDS`:

```
desenvolvedor, engenheiro, .net, c#
```

Customizável como as outras:

```
BTG_KEYWORDS=Desenvolvedor,Pleno,.NET,C#,React,Azure
```

## E-mail de status (heartbeat)

Como o alerta só chega quando surge vaga nova, pode passar dias sem
notificação nenhuma — o que dificulta saber se o agendamento ainda está
funcionando ou se quebrou silenciosamente. Pra resolver isso, se ficar mais
de `HEARTBEAT_HOURS` horas (default 24) sem nenhuma vaga nova, a próxima
execução manda um e-mail curto avisando "ainda funcionando, sem vaga nova".
Um e-mail de vaga nova também conta como heartbeat (reinicia a contagem), já
que ele já prova que o sistema está de pé.

Isso é controlado pela env var `HEARTBEAT_HOURS` — ajuste ou desative
(colocando um valor bem alto) conforme sua preferência.

## Rodando localmente

```bash
cp .env.example .env
# edite o .env com suas credenciais de SMTP (e opcionalmente KEYWORDS)
go mod tidy
go run .
```

O programa carrega o `.env` automaticamente (via `godotenv`) quando não está
rodando em Lambda — não precisa exportar as variáveis manualmente antes.

Sem `S3_BUCKET` definido, o histórico de vagas vistas fica em `seen_jobs.json`
local — mesmo comportamento de antes, bom pra testar.

Se aparecer "nenhuma vaga nova", rode de novo depois de apagar
`seen_jobs.json` pra forçar detectar tudo que existe hoje como "novo" (útil
pra testar o envio de e-mail).

## Configurando o e-mail (Gmail)

O jeito mais simples pra testar é usar uma **senha de app** do Gmail
(não é a sua senha normal):

1. Ative a verificação em duas etapas na sua conta Google.
2. Acesse myaccount.google.com/apppasswords e gere uma senha de app.
3. Use essa senha em `SMTP_PASS` no `.env`.

Pra algo mais robusto/escalável depois, dá pra trocar por **Amazon SES**
(faz sentido já que você vai rodar na AWS mesmo) —亦 aceita autenticação SMTP
parecida, só muda host/porta/credenciais.

## Rodando na AWS (Lambda + EventBridge)

O programa detecta automaticamente que está rodando em Lambda (via a env var
`AWS_LAMBDA_FUNCTION_NAME`, setada pelo runtime) e usa `lambda.Start(...)` em
vez de rodar uma vez e sair. Como Lambda não tem disco persistente entre
execuções, o histórico de vagas vistas (`seen_jobs.json`) é lido/salvo no
**S3** em vez do disco local — configurado via `S3_BUCKET`/`S3_KEY`.

### 1. Criar o bucket S3 pro histórico

```bash
aws s3 mb s3://SEU-BUCKET-AQUI --region sa-east-1
```

### 2. Compilar o binário pra Lambda (runtime `provided.al2023`)

O handler do `aws-lambda-go` espera um binário chamado `bootstrap`:

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap .
zip deployment.zip bootstrap
```

(`arm64` = arquitetura Graviton, mais barata; troque `GOARCH=amd64` se preferir x86.)

### 3. Criar a função Lambda

```bash
aws lambda create-function \
  --function-name itau-job-alert \
  --runtime provided.al2023 \
  --architectures arm64 \
  --handler bootstrap \
  --zip-file fileb://deployment.zip \
  --role arn:aws:iam::SUA_CONTA:role/itau-job-alert-role \
  --timeout 60 \
  --memory-size 128 \
  --environment "Variables={SMTP_HOST=smtp.gmail.com,SMTP_PORT=587,SMTP_USER=seuemail@gmail.com,SMTP_PASS=sua_senha_de_app,EMAIL_FROM=seuemail@gmail.com,EMAIL_TO=seuemail@gmail.com,S3_BUCKET=SEU-BUCKET-AQUI,KEYWORDS=Engenheiro,Engenharia,Java,.NET,Desenvolvedor,Junior,Pleno,Senior}"
```

A role (`itau-job-alert-role`) precisa da policy gerenciada
`AWSLambdaBasicExecutionRole` (logs no CloudWatch) mais permissão de
`s3:GetObject`/`s3:PutObject` no bucket criado no passo 1.

### 4. Agendar com EventBridge (a cada 15 min)

```bash
aws events put-rule \
  --name itau-job-alert-schedule \
  --schedule-expression "rate(15 minutes)"

aws lambda add-permission \
  --function-name itau-job-alert \
  --statement-id eventbridge-invoke \
  --action lambda:InvokeFunction \
  --principal events.amazonaws.com \
  --source-arn arn:aws:events:sa-east-1:SUA_CONTA:rule/itau-job-alert-schedule

aws events put-targets \
  --rule itau-job-alert-schedule \
  --targets "Id"="1","Arn"="arn:aws:lambda:sa-east-1:SUA_CONTA:function:itau-job-alert"
```

### Redeployando depois de mudar o código

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap .
zip deployment.zip bootstrap
aws lambda update-function-code --function-name itau-job-alert --zip-file fileb://deployment.zip
```

## Sobre custo

Esse programa roda por poucos segundos a cada execução, então Lambda é bem
mais barato/idiomático que deixar uma EC2 ligada 24/7 só esperando o timer:
você paga só pelos segundos de execução, e pra esse volume (a cada 15 min =
~2880 invocações/mês) fica dentro do free tier "pra sempre" da AWS (1M
invocações/mês grátis). O S3 pro histórico também é irrisório (um arquivo de
poucos KB).

## Próximos passos possíveis

- Trocar o e-mail por **Amazon SES** (autenticação SMTP parecida, só muda
  host/porta/credenciais) — evita depender de senha de app do Gmail.
- Adicionar mais empresas (cada uma provavelmente precisa de um fetch/parser
  próprio, já que cada site de carreira tem estrutura ou plataforma
  diferente — o Itaú tem site próprio, o PicPay usa Oracle Cloud HCM, o BTG
  usa Greenhouse). Antes de integrar uma nova, vale checar se ela usa
  Greenhouse (`boards-api.greenhouse.io/v1/boards/<token>/jobs`) — é a API
  pública oficialmente documentada, mais simples e estável de integrar.
- Alertar via Slack/Telegram além de (ou em vez de) e-mail.

## Sobre as fontes de vaga (fora o Itaú)

**PicPay** — diferente do Itaú (scraping de HTML do site próprio), usa
Oracle Cloud HCM e as vagas são buscadas via `fetchPicPayJobs` (`picpay.go`)
num endpoint JSON público — o mesmo que a página de carreiras deles usa por
trás dos panos. Não é uma API oficialmente documentada pra esse uso, então
se o PicPay trocar de plataforma de novo (já saíram da Gupy em algum
momento), essa busca quebra sem aviso.

**BTG Pactual** — usa Greenhouse, e as vagas são buscadas via `fetchBTGJobs`
(`btg.go`) na API pública **oficialmente documentada**
(`developers.greenhouse.io/job-board.html`) — mais estável que a do PicPay,
já que é pensada pra esse tipo de integração externa.
