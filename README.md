```shell
curl -SL https://pscli.boxlayer.com -o /usr/local/bin/pscli && \
chmod +x /usr/local/bin/pscli
```

```shell
docker login --username myname boxlayer.com
docker pull boxlayer.com/official/base
pscli . boxlayer.com/my-username/image-name
```
