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

## Standard Linksharing with Uplink
Anything shared with `--url` will be readonly and available publicly (no secret key needed).

`uplink share --url sj://<path>`

results in

`https://link.tardigradeshare.io/jqaz8xihdea93jfbaks8324jrhq1/<path>`

## Custom URL configuration and static site hosting with Uplink

You can use your own domain and host your website on Tardigrade with the following setup.

0. Upload your static site and other files to tardigrade using [Uplink](https://documentation.tardigrade.io/getting-started/uploading-your-first-object/set-up-uplink-cli) 
or [S3 gateway](https://documentation.tardigrade.io/api-reference/s3-gateway). Download the [Uplink Binary](https://documentation.tardigrade.io/getting-started/uploading-your-first-object/set-up-uplink-cli). 
   

1. Share an object or path to an object. 
   If you are sharing an entire bucket or sub-folder, you will want to name your home page index.html.
   Anything shared with `--dns` will be readonly and available publicly (no secret key needed).
   
   `uplink share --dns <hostname> sj://<path>`

   Prints a zone file with the information needed to create 3 dns records. Remember to update the $ORIGIN with your domain name. You may also change the $TTL.

   ```
   $ORIGIN example.com.
   $TTL    3600
   <hostname>    	IN	CNAME	link.tardigradeshare.io.
   txt-<hostname> 	IN	TXT  	storj-root:<path>
   txt-<hostname> 	IN	TXT  	storj-access:<access key>
   ```
   For example `uplink share --dns www sj://bucket/prefix` will output:
   ```
   $ORIGIN example.com.
   $TTL    3600
   www    	IN	CNAME	link.tardigradeshare.io.
   txt-www	IN	TXT  	storj-root:bucket/prefix
   txt-www	IN	TXT  	storj-access:jqaz8xihdea93jfbaks8324jrhq1
   ```

2. Create a CNAME record on your hostname using our linksharing common URL `link.tardigradeshare.io.` as the target name.
 
    <img src="docs/images/cname.png" width="50%">

3. Create 2 TXT records, prepending `txt-` to your hostname.
    
    a. Root Path: the bucket, object prefix key, or individual object that you want your root domain to resolve to.
    
    <img src="docs/images/root.png" width="50%">
    
    b. Access Key: the readonly and public access key to your root path.
 
    <img src="docs/images/access.png" width="50%">

4. You can check to make sure your dns records are ready with `dig @1.1.1.1 txt-<hostname>.<domain> TXT`        
    
5. Without further action, your site will be served with http. You can secure your site by using a https proxy server such as [Cloudflare](https://www.cloudflare.com/)

6. That's it! You should be all set to access your website e.g. `http://www.example.test`

[Maxmind]: https://dev.maxmind.com/geoip/geoipupdate/