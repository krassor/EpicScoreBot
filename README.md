# EpicScoreBot

Telegram bot for scoring development epics and risks.

## Build & Run

```bash
# Local
go build -o bin/epicScoreBot app/main.go
./bin/epicScoreBot --config config/config.yml

# Docker
docker-compose up --build
```

## Configuration

Edit `config/config.yml` with your database and Telegram bot settings.

## Bot Commands

**Admin:**
- `/addteam <name>` — create a team
- `/adduser <tgID> <firstName> <lastName> <weight>` — register a user
- `/assignrole <tgID> <roleName>` — assign a role
- `/assignteam <tgID> <teamName>` — assign to a team
- `/addepic <team> | <number> | <name> | <description>` — create an epic
- `/addrisk <epicNumber> | <description>` — add a risk
- `/startscore <epicNumber>` — send epic for scoring
- `/results <epicNumber>` — view results

**Users:**
- `/score` — open scoring menu
