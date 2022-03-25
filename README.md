# sshproxy

This is a PoC ssh proxy written in Go. It is meant as a toy
proxy to play around with and help me figure out how exactly 
the ssh protocol works.

Except for the obvious lack of security (it accepts any public key)
the code should be resonably ready to be plopped into production.

## How it works

It binds to port 4222. On a successful authentication it will 
ssh into the destination (localhost:3222) and then proxy the connection.

## docker image.

## Alternative approach to proxying

This approach includes rather protocol-intensive proxying. I'm not familiar enough 
with SSH to know whether this could be done simpler, perhaps by copying the decrypted bytes
coming from the one connection to the next.

## Notable ssh proxies in Go

Other proxies and ssh implementations to look at:
 * https://github.com/appleboy/easyssh-proxy
 * https://github.com/blacknon/go-sshlib 
 * https://github.com/tsurubee/sshr
 * https://github.com/gliderlabs/ssh 



