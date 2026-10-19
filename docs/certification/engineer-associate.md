# Yandex Cloud Certified Engineer Associate — study kit

Russian translation: [engineer-associate.ru.md](engineer-associate.ru.md). This file is canonical.

A study kit for exam preparation, tied to the **MOEX Pulse** project
(`docs/plans/20260927-moex-pulse.md`). The idea: every project task is built while reading the
matching section of the materials, so practice and preparation go together.

## Exam facts

- Level: basic. Format: **65 questions online, 90 minutes**, pass mark **70%**.
- Price: **7 000 RUB**. Proctoring through **Examus** (you need a second camera on a phone,
  a clear desk, background applications switched off; check everything in advance).
- The **exam is theoretical** (not labs), but with a strong emphasis on **sequences of CLI commands**,
  rarely used service options and exact wording. Ambiguous questions occasionally occur.
- Strategy in 3 passes: confident answers first → then hard ones → a final check.
- Prepare for at least a week; keep the knowledge "fresh" (review right before the exam).
- Registration: https://app.examus.net/yandex-cloud · info: https://yandex.cloud/ru/certification

## Priority gaps (according to those who passed)

Spend more time where there is less practice:
- **CLI commands** (`yc ...`) — even if you are used to GUI/Terraform, the exam asks about the CLI.
- Networking (VPC, security groups, routing), **CDN** and Big Data.
- **DataLens** (BI visualization).
- **Lockbox** (secrets management).
- **Consumption/billing calculation**.

## Official materials (free unless marked)

- **Free course "Yandex Cloud Fundamentals"** (with a certificate) — the basics: https://yandex.cloud/ru/training
- **Documentation** for every service + practical guides (tutorials): https://yandex.cloud/ru/docs
- **Yandex Cloud YouTube channel** — service walkthroughs.
- **Learning paths** in the training hub: Cloud Base, DevOps, Data, Security.
- Telegram: certification preparation https://t.me/YCCertification · courses https://t.me/yc_courses
- Competencies/blueprint: https://yandex.cloud/ru/certification/engineer/competencies
- FAQ/rules: https://yandex.cloud/ru/certification/faq · https://yandex.cloud/ru/certification/requirements
- **Paid, optional:** the course "Cloud Services Engineer" (Yandex Practicum) —
  practical tasks in the cloud: https://practicum.yandex.ru/ycloud

## Blueprint: domains → services → MOEX Pulse task

### 1. Infrastructure
Compute Cloud (VMs, snapshots/images), VPC, Network/Application Load Balancer,
security groups, instance groups + autoscaling.
→ *MOEX Pulse:* Phase 2 (k8s nodes, ALB, instance groups); VPC — Task 8.

### 2. Data and storage
Object Storage (classes, access), Managed PostgreSQL/MySQL, **YDB**, **ClickHouse**,
OpenSearch, **Managed Kafka**, Data Proc, Data Transfer, **DataLens**, DataSphere.
→ *MOEX Pulse:* YDB + Object Storage (Task 5–8); ClickHouse + DataLens + Kafka + Data Transfer (Phase 2).

### 3. DevOps and automation
Yandex Cloud CLI, **Terraform**, Docker/**Container Registry**, **Managed Kubernetes**,
Managed GitLab, **Monitoring** (custom metrics).
→ *MOEX Pulse:* Terraform + Container Registry + Monitoring (Task 8); k8s (Phase 2).
Separately run the CLI equivalents of everything you do in Terraform (for the exam).

### 4. Serverless
**Cloud Functions**, **API Gateway**, **Message Queue**, Data Streams, Serverless Containers,
Serverless YDB.
→ *MOEX Pulse:* Functions + API Gateway + Serverless YDB — the core of Phase 1 (Task 5–8);
Message Queue/Data Streams (Phase 2).

### 5. Security
**IAM**, **KMS**, **Lockbox**, Certificate Manager.
→ *MOEX Pulse:* IAM (least-privilege SA) + KMS + Lockbox — Task 8; Certificate Manager (Phase 2, TLS on the ALB).

### 6. Billing
Cost calculation, billing account, cost optimization.
→ *MOEX Pulse:* estimate the cost of every service when provisioning; watch the sandbox spend.

## Preparation checklist

- [ ] Complete the free course "Yandex Cloud Fundamentals".
- [ ] For each domain read the docs of the chosen services + one tutorial.
- [ ] For every service I bring up in MOEX Pulse, write down the **CLI equivalent** (`yc ...`).
- [ ] Close the gaps separately: networking/CDN, DataLens, Lockbox, billing calculation.
- [ ] Go through sample questions/discussions in @YCCertification.
- [ ] Set up and test Examus (second camera, clear desk) in advance.
- [ ] Keep a "domain → service → touched in practice" table as the project is built.

## Sources

- https://yandex.cloud/ru/certification
- https://yandex.cloud/ru/certification/engineer/competencies
- https://yandex.cloud/ru/training
- https://habr.com/ru/companies/yandex_cloud_and_infra/articles/827412/
- https://practicum.yandex.ru/ycloud
