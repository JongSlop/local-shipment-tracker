We want a simple, self-hosted, local service that does the following:
- No auth
- Responsive design
- Tracks shipments from all leading german logistics providers (DHL/DeutschePost, DPD, GLS, myHermes, maybe Amazon if they expose an API)
- Easy adding new shipments in the UI via tracking number
- View what arrives soon at a glance
  
Deployment via Docker stack on a Synology NAS.  
Repo remote is `git@github.com:JongSlop/local-shipment-tracker.git`.  
Stack should be simple and performant, i am thinking golang.
