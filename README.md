# Link Sharing Service

## Building

```
$ go install storj.io/linksharing
```

## Configuring

### Development

Default development configuration has the link sharing service hosted on
`localhost:8080` serving plain HTTP.

```
$ linksharing setup --defaults dev
```

### Production

To configure the link sharing service for production, run the `setup` command
using the `release` defaults. An required argument is the location of the geo-location database.
You must also provide the public URL for the sharing service, which is used to construct URLs returned to
clients. Since there is currently no server affinity for requests, the URL
can point to a pool of servers:

```
$ linksharing setup --defaults release --geo-location-db <PATH TO FILE> --public-url <PUBLIC URL> 
```

**NOTE**: Please follow this link for instructions how to install/download the geo-location database:
https://dev.maxmind.com/geoip/geoipupdate/

Default release configuration has the link sharing service hosted on `:8443`
serving HTTPS using a server certificate (`server.crt.pem`) and
key (`server.key.pem`) residing in the working directory where the linksharing
service is run.

You can modify the configuration file or use the `--cert-file` and `--key-file`
flags to configure an alternate location for the server keypair.

In order to run the link sharing service in release mode serving HTTP, you must
clear the certificate and key file configurables:

```
$ linksharing setup --defaults release --public-url <PUBLIC URL> --cert-file="" --key-file="" --address=":8080"
```

**WARNING** HTTP is only recommended if you are doing TLS termination on the
same machine running the link sharing service as the link sharing service
serves unencrypted user data.

## Running

After configuration is complete, running the link sharing is as simple as:

```
$ linksharing run
```

## Custom URL configuration and static site hosting

You can use your own domain and host your website on tardigrade with the following setup.

0. Upload your static site and other files to tardigrade using [Uplink](https://github.com/storj/storj/wiki/Uplink-CLI) 
or [S3 gateway](https://documentation.tardigrade.io/api-reference/s3-gateway). Download the [Uplink Binary](https://github.com/storj/storj/wiki/Uplink-CLI).

1. Share your READONLY objects via `uplink share --readonly --dns <hostname> sj://<bucket/prefix>`.

This command will provide the info needed to create your 3 dns records.

For example `uplink share --dns yourHostName sj://bucket/prefix` will output:

```
Type CNAME, Hostname yourHostName, Target link.tardigradeshare.io.
------
Type TXT, Hostname txt-yourHostName, Content storj-root:bucket/prefix
------
Type TXT, Hostname txt-yourHostName, Content storj-access:3WXG1qE
```

2. Create a CNAME record on your hostname using our linksharing common URL `link.tardigradeshare.io.` as the target name.
   
    `Type CNAME, Hostname yourHostName, Target link.tardigradeshare.io.`
    
    <img src="docs/images/cname.png" width="50%">

3. Create 2 TXT records, prepending `txt-` to your hostname.
    
    a. Root Path: the bucket, object prefix key, or individual object that you want your root domain to resolve to.
   
    `Type TXT, Hostname txt-yourHostName, Content storj-root:bucket/prefix`
    
    <img src="docs/images/root.png" width="50%">
    
    b. Access Key: the readonly and public access key to your root path.
    
    `Type TXT, Hostname txt-yourHostName, Content storj-access:3WXG1qE`
    
    <img src="docs/images/access.png" width="50%">

4. You can check to make sure your dns records are ready with `dig @1.1.1.1 txt-<yourHostName>.<yourDomain> TXT`        
    
5. Without further action, your site will be served with http. You can secure your site by using a https proxy server such as [Cloudflare](https://www.cloudflare.com/)

6. That's it! You should be all set to access your website e.g. `http://yourHostName.yourwebsite.com`

[Maxmind]: https://dev.maxmind.com/geoip/geoipupdate/