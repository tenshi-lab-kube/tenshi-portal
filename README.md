# Tenshi Portal

Go web portal for Tenshi Lab cluster services.

Suggested public hostname:

```text
home.tenshi-lab.fr -> http://tenshi-portal.tenshi-portal.svc.cluster.local:8080
```

Services linked by default:

- Authentik: https://auth.tenshi-lab.fr
- Grafana: https://grafana.tenshi-lab.fr
- Argo CD: https://argocd.tenshi-lab.fr
- Discord Tickets: https://tickets.tenshi-lab.fr

Deploy with Argo CD:

```bash
kubectl apply -f https://raw.githubusercontent.com/tenshi-lab-kube/tenshi-portal/main/argocd/application.yaml
```
