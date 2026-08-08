# GitHub → Azure auto-deploy (OIDC)

Push to `main` runs `.github/workflows/deploy.yml`:
1. `make verify`
2. `az acr build` → ACR
3. `az containerapp update` → live API

Your UCLA Azure AD account **cannot create app registrations via CLI**
(`Insufficient privileges`). Create the GitHub deploy identity once in the
portal (or ask Anderson IT), then add three GitHub secrets.

## 1. Create an App Registration (portal)

1. Open [Entra ID → App registrations](https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade)
2. **New registration** → name `jokefactory-gh-deploy` → **Accounts in this organizational directory only** → Register
3. Copy **Application (client) ID** and **Directory (tenant) ID**

## 2. Federated credential (for GitHub OIDC)

On that app → **Certificates & secrets** → **Federated credentials** → **Add**:

| Field | Value |
|-------|--------|
| Scenario | GitHub Actions deploying Azure resources |
| Organization | `Hussain-Khozema` |
| Repository | `jokefactory_be` |
| Entity | Branch |
| Branch | `main` |
| Name | `github-main` |

## 3. Create a service principal + roles

In Cloud Shell / local terminal (after the app exists):

```bash
APP_ID="<Application client ID from step 1>"
SUB="32586e37-da18-4a45-a0ca-1912b31d814a"
RG=ai-joke-factory
ACR=aijokefactoryacr

# Create SP for the app (no-op if it already exists)
az ad sp create --id "$APP_ID"

SP_OID=$(az ad sp show --id "$APP_ID" --query id -o tsv)
ACR_ID=$(az acr show -n "$ACR" -g "$RG" --query id -o tsv)
RG_ID=$(az group show -n "$RG" --query id -o tsv)

az role assignment create --assignee-object-id "$SP_OID" --assignee-principal-type ServicePrincipal \
  --role AcrPush --scope "$ACR_ID"

az role assignment create --assignee-object-id "$SP_OID" --assignee-principal-type ServicePrincipal \
  --role Contributor --scope "$RG_ID"
```

If `az ad sp create` is also forbidden, ask IT to create the SP and grant those two roles on RG `ai-joke-factory`.

## 4. GitHub secrets

```bash
gh secret set AZURE_CLIENT_ID --body "<Application client ID>"
gh secret set AZURE_TENANT_ID --body "dfaafa2f-06e7-4592-8d47-c6e9e3eabb15"
gh secret set AZURE_SUBSCRIPTION_ID --body "32586e37-da18-4a45-a0ca-1912b31d814a"
```

Or: repo → **Settings → Secrets and variables → Actions → New repository secret**.

## 5. Ship it

Merge/push the workflow to `main`, then either push a commit or:

```bash
gh workflow run deploy.yml
```

Watch: **Actions** tab → **deploy**.

Live URL after success:
`https://jokefactory-api.whitepebble-2daa4226.westus2.azurecontainerapps.io`
