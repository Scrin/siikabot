# SiikaBot

A simple [Matrix](https://matrix.org) bot experiment.

This is not intended to be used by others and I don't expect anyone to want to run their own instance of this, as this is just a collection of random features on a bot I wanted to make. I'll be making breaking changes without warning and I won't be really writing any real documentation either, except for small bits that are more intended as notes for myself.

For local development, a postgres container can be brought up with:

```sh
docker run --rm --name siikabot-dev-postgres -p 5432:5432 -e POSTGRES_PASSWORD=password -v "$(pwd)/postgres_data:/var/lib/postgresql/data" postgres -c log_statement=all
```
