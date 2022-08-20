---
title: Penny Vault API v1.0.0
language_tabs:
  - shell: Shell
  - http: HTTP
  - javascript: JavaScript
  - ruby: Ruby
  - python: Python
  - php: PHP
  - java: Java
  - go: Go
toc_footers: []
includes: []
search: false
highlight_theme: darkula
headingLevel: 2

---

<!-- Generator: Widdershins v4.0.1 -->

<h1 id="penny-vault-api">Penny Vault API v1.0.0</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

Execute investment strategies and compute portfolio metrics

Base URLs:

* <a href="http://localhost:3000/v1">http://localhost:3000/v1</a>

* <a href="https://penny-vault.herokuapp.com/v1">https://penny-vault.herokuapp.com/v1</a>

* <a href="https://pv-api-beta.herokuapp.com/v1">https://pv-api-beta.herokuapp.com/v1</a>

# Authentication

- HTTP Authentication, scheme: bearer 

* API Key (ApiKeyAuth)
    - Parameter Name: **apikey**, in: query. 

<h1 id="penny-vault-api-utility">utility</h1>

## Ping service

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/ \
  -H 'Accept: application/json'

```

```http
GET http://localhost:3000/v1/ HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://localhost:3000/v1/',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://localhost:3000/v1/', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /`

ping api to check for liveness and wake-up any sleeping instances

> Example responses

> 200 Response

```json
{
  "message": "API is alive",
  "status": "success",
  "time": "2021-06-19T08:09:10.115924-05:00"
}
```

<h3 id="ping-service-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[PingResponse](#schemapingresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|ERROR|None|

<aside class="success">
This operation does not require authentication
</aside>

<h1 id="penny-vault-api-security">security</h1>

## list securities

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/security \
  -H 'Accept: application/json' \
  -H 'range: items=0-9'

```

```http
GET http://localhost:3000/v1/security HTTP/1.1
Host: localhost:3000
Accept: application/json
range: items=0-9

```

```javascript

const headers = {
  'Accept':'application/json',
  'range':'items=0-9'
};

fetch('http://localhost:3000/v1/security',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json',
  'range' => 'items=0-9'
}

result = RestClient.get 'http://localhost:3000/v1/security',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json',
  'range': 'items=0-9'
}

r = requests.get('http://localhost:3000/v1/security', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
    'range' => 'items=0-9',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/security', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/security");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
        "range": []string{"items=0-9"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/security", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /security`

<h3 id="list-securities-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|q|query|string|false|query string|
|range|header|string|false|range header specifying which items should be returned; limit of 100 items at a time|

> Example responses

> 200 Response

```json
{
  "compositeFigi": "string",
  "cusip": "string",
  "name": "string",
  "ticker": "string"
}
```

<h3 id="list-securities-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Security](#schemasecurity)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|
|416|[Range Not Satisfiable](https://tools.ietf.org/html/rfc7233#section-4.4)|Unallowable range|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

<h1 id="penny-vault-api-portfolio">portfolio</h1>

## retrieve a list of portfolios for the logged-in user

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/portfolio/ \
  -H 'Accept: application/json'

```

```http
GET http://localhost:3000/v1/portfolio/ HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/portfolio/',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://localhost:3000/v1/portfolio/',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://localhost:3000/v1/portfolio/', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/portfolio/', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/portfolio/");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/portfolio/", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /portfolio/`

> Example responses

> 200 Response

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}
```

<h3 id="retrieve-a-list-of-portfolios-for-the-logged-in-user-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Portfolio](#schemaportfolio)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

## create a new portfolio

> Code samples

```shell
# You can also use wget
curl -X POST http://localhost:3000/v1/portfolio/ \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json'

```

```http
POST http://localhost:3000/v1/portfolio/ HTTP/1.1
Host: localhost:3000
Content-Type: application/json
Accept: application/json

```

```javascript
const inputBody = '{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/portfolio/',
{
  method: 'POST',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json'
}

result = RestClient.post 'http://localhost:3000/v1/portfolio/',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json'
}

r = requests.post('http://localhost:3000/v1/portfolio/', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('POST','http://localhost:3000/v1/portfolio/', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/portfolio/");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("POST");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("POST", "http://localhost:3000/v1/portfolio/", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`POST /portfolio/`

> Body parameter

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}
```

<h3 id="create-a-new-portfolio-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[Portfolio](#schemaportfolio)|true|New portfolio settings|

> Example responses

> 200 Response

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800
}
```

<h3 id="create-a-new-portfolio-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Portfolio](#schemaportfolio)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|

<aside class="success">
This operation does not require authentication
</aside>

## retrieve a specific portfolio

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/portfolio/{id} \
  -H 'Accept: application/json'

```

```http
GET http://localhost:3000/v1/portfolio/{id} HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/portfolio/{id}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://localhost:3000/v1/portfolio/{id}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://localhost:3000/v1/portfolio/{id}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/portfolio/{id}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/portfolio/{id}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/portfolio/{id}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /portfolio/{id}`

<h3 id="retrieve-a-specific-portfolio-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string(uuid)|true|the portfolio id to retrieve|

> Example responses

> 200 Response

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}
```

<h3 id="retrieve-a-specific-portfolio-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Portfolio](#schemaportfolio)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|No portoflio found for specified id|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

## modify a portfolio

> Code samples

```shell
# You can also use wget
curl -X PATCH http://localhost:3000/v1/portfolio/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json'

```

```http
PATCH http://localhost:3000/v1/portfolio/{id} HTTP/1.1
Host: localhost:3000
Content-Type: application/json
Accept: application/json

```

```javascript
const inputBody = '{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}';
const headers = {
  'Content-Type':'application/json',
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/portfolio/{id}',
{
  method: 'PATCH',
  body: inputBody,
  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Content-Type' => 'application/json',
  'Accept' => 'application/json'
}

result = RestClient.patch 'http://localhost:3000/v1/portfolio/{id}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Content-Type': 'application/json',
  'Accept': 'application/json'
}

r = requests.patch('http://localhost:3000/v1/portfolio/{id}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Content-Type' => 'application/json',
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('PATCH','http://localhost:3000/v1/portfolio/{id}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/portfolio/{id}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("PATCH");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Content-Type": []string{"application/json"},
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("PATCH", "http://localhost:3000/v1/portfolio/{id}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`PATCH /portfolio/{id}`

> Body parameter

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}
```

<h3 id="modify-a-portfolio-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string(uuid)|true|the portfolio id to update|
|body|body|[Portfolio](#schemaportfolio)|true|portfolio settings to update|

> Example responses

> 200 Response

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "My Portfolio Renamed",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0.32,
    "Valid": true
  },
  "cagrSinceInception": {
    "Float64": 0.14323,
    "Valid": true
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}
```

<h3 id="modify-a-portfolio-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Portfolio](#schemaportfolio)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

## delete the portfolio

> Code samples

```shell
# You can also use wget
curl -X DELETE http://localhost:3000/v1/portfolio/{id} \
  -H 'Accept: application/json'

```

```http
DELETE http://localhost:3000/v1/portfolio/{id} HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/portfolio/{id}',
{
  method: 'DELETE',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.delete 'http://localhost:3000/v1/portfolio/{id}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.delete('http://localhost:3000/v1/portfolio/{id}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('DELETE','http://localhost:3000/v1/portfolio/{id}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/portfolio/{id}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("DELETE");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("DELETE", "http://localhost:3000/v1/portfolio/{id}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`DELETE /portfolio/{id}`

<h3 id="delete-the-portfolio-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string(uuid)|true|the portfolio id to delete|

> Example responses

> 200 Response

```json
{
  "status": "success"
}
```

<h3 id="delete-the-portfolio-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Status](#schemastatus)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

<h1 id="penny-vault-api-strategy">strategy</h1>

## retrieve a list of strategies

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/strategy/ \
  -H 'Accept: application/json'

```

```http
GET http://localhost:3000/v1/strategy/ HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/strategy/',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://localhost:3000/v1/strategy/',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://localhost:3000/v1/strategy/', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/strategy/', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/strategy/");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/strategy/", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /strategy/`

> Example responses

> 200 Response

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}
```

<h3 id="retrieve-a-list-of-strategies-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[Portfolio](#schemaportfolio)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

## retrieve details about a specific strategy

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/strategy/{shortcode} \
  -H 'Accept: application/json'

```

```http
GET http://localhost:3000/v1/strategy/{shortcode} HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/strategy/{shortcode}',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://localhost:3000/v1/strategy/{shortcode}',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://localhost:3000/v1/strategy/{shortcode}', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/strategy/{shortcode}', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/strategy/{shortcode}");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/strategy/{shortcode}", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /strategy/{shortcode}`

<h3 id="retrieve-details-about-a-specific-strategy-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|shortcode|path|string|true|shortcode of the strategy to retrieve|

> Example responses

> 200 Response

```json
{
  "name": "string",
  "shortcode": "string",
  "benchmark": "string",
  "description": "string",
  "longDescription": "string",
  "source": "string",
  "version": "1.0.0",
  "arguments": {},
  "suggestedParams": {},
  "metrics": {
    "cagrs": {
      "1-yr": 0.3811364073876089,
      "10-yr": 0.14472186517787256,
      "3-yr": 0.17810447557991638,
      "5-yr": 0.17070350884131757
    },
    "drawDowns": {
      "begin": 1196380800,
      "end": 1235692800,
      "lossPercent": -0.5096921151209781,
      "recovery": 1346371200
    },
    "sharpeRatio": 0.5691295208327964,
    "sortinoRatio": 0.8399589527305837,
    "stdDev": 0.14508276256543967,
    "ulcerIndexAvg": 11.339463389310689
  }
}
```

<h3 id="retrieve-details-about-a-specific-strategy-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[StrategyInfo](#schemastrategyinfo)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|No portoflio found for specified id|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

## execute the strategy with given parameters

> Code samples

```shell
# You can also use wget
curl -X GET http://localhost:3000/v1/strategy/{shortcode}/execute \
  -H 'Accept: application/json'

```

```http
GET http://localhost:3000/v1/strategy/{shortcode}/execute HTTP/1.1
Host: localhost:3000
Accept: application/json

```

```javascript

const headers = {
  'Accept':'application/json'
};

fetch('http://localhost:3000/v1/strategy/{shortcode}/execute',
{
  method: 'GET',

  headers: headers
})
.then(function(res) {
    return res.json();
}).then(function(body) {
    console.log(body);
});

```

```ruby
require 'rest-client'
require 'json'

headers = {
  'Accept' => 'application/json'
}

result = RestClient.get 'http://localhost:3000/v1/strategy/{shortcode}/execute',
  params: {
  }, headers: headers

p JSON.parse(result)

```

```python
import requests
headers = {
  'Accept': 'application/json'
}

r = requests.get('http://localhost:3000/v1/strategy/{shortcode}/execute', headers = headers)

print(r.json())

```

```php
<?php

require 'vendor/autoload.php';

$headers = array(
    'Accept' => 'application/json',
);

$client = new \GuzzleHttp\Client();

// Define array of request body.
$request_body = array();

try {
    $response = $client->request('GET','http://localhost:3000/v1/strategy/{shortcode}/execute', array(
        'headers' => $headers,
        'json' => $request_body,
       )
    );
    print_r($response->getBody()->getContents());
 }
 catch (\GuzzleHttp\Exception\BadResponseException $e) {
    // handle exception or api errors.
    print_r($e->getMessage());
 }

 // ...

```

```java
URL obj = new URL("http://localhost:3000/v1/strategy/{shortcode}/execute");
HttpURLConnection con = (HttpURLConnection) obj.openConnection();
con.setRequestMethod("GET");
int responseCode = con.getResponseCode();
BufferedReader in = new BufferedReader(
    new InputStreamReader(con.getInputStream()));
String inputLine;
StringBuffer response = new StringBuffer();
while ((inputLine = in.readLine()) != null) {
    response.append(inputLine);
}
in.close();
System.out.println(response.toString());

```

```go
package main

import (
       "bytes"
       "net/http"
)

func main() {

    headers := map[string][]string{
        "Accept": []string{"application/json"},
    }

    data := bytes.NewBuffer([]byte{jsonReq})
    req, err := http.NewRequest("GET", "http://localhost:3000/v1/strategy/{shortcode}/execute", data)
    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    // ...
}

```

`GET /strategy/{shortcode}/execute`

<h3 id="execute-the-strategy-with-given-parameters-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|shortcode|path|string|true|shortcode of the strategy to run|
|arguments|query|string|false|JSON arguments for specified strategy|
|startDate|query|string(date)|false|date to begin simulation on|
|endDate|query|string(date)|false|date to finish simulation on|

> Example responses

> 200 Response

```json
{
  "cagrSinceInception": 0,
  "computedOn": 0,
  "currentAsset": "VFINX",
  "measurements": [
    {
      "justification": {},
      "percentReturn": 0,
      "riskFreeValue": 10000,
      "time": 617846400,
      "value": 10000
    }
  ],
  "metrics": [
    {
      "cagrs": {
        "1-yr": 0.3811364073876089,
        "10-yr": 0.14472186517787256,
        "3-yr": 0.17810447557991638,
        "5-yr": 0.17070350884131757
      },
      "drawDowns": {
        "begin": 1196380800,
        "end": 1235692800,
        "lossPercent": -0.5096921151209781,
        "recovery": 1346371200
      },
      "sharpeRatio": 0.5691295208327964,
      "sortinoRatio": 0.8399589527305837,
      "stdDev": 0.14508276256543967,
      "ulcerIndexAvg": 11.339463389310689
    }
  ],
  "periodEnd": 0,
  "periodStart": 0,
  "totalDeposited": 0,
  "totalWithdrawn": 0,
  "transactions": [
    {
      "date": "1989-07-31T00:00:00Z",
      "justification": {},
      "kind": "DEPOSIT",
      "pricePerShare": 1,
      "shares": 10000,
      "ticker": "$CASH",
      "totalValue": 10000
    }
  ],
  "ytdReturn": 0
}
```

<h3 id="execute-the-strategy-with-given-parameters-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|OK|[PortfolioPerformance](#schemaportfolioperformance)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|Bad parameters|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Not Authorized|None|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|No strategy found for specified shortcode|None|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None
</aside>

# Schemas

<h2 id="tocS_Holding">Holding</h2>
<!-- backwards compatibility -->
<a id="schemaholding"></a>
<a id="schema_Holding"></a>
<a id="tocSholding"></a>
<a id="tocsholding"></a>

```json
{
  "ticker": "VFINX",
  "shares": 10,
  "percentPortfolio": 1,
  "value": 10000
}

```

A holding in the portfolio

### Properties

*None*

<h2 id="tocS_MetricBundle">MetricBundle</h2>
<!-- backwards compatibility -->
<a id="schemametricbundle"></a>
<a id="schema_MetricBundle"></a>
<a id="tocSmetricbundle"></a>
<a id="tocsmetricbundle"></a>

```json
{
  "cagrs": {
    "1-yr": 0.3811364073876089,
    "10-yr": 0.14472186517787256,
    "3-yr": 0.17810447557991638,
    "5-yr": 0.17070350884131757
  },
  "drawDowns": {
    "begin": 1196380800,
    "end": 1235692800,
    "lossPercent": -0.5096921151209781,
    "recovery": 1346371200
  },
  "sharpeRatio": 0.5691295208327964,
  "sortinoRatio": 0.8399589527305837,
  "stdDev": 0.14508276256543967,
  "ulcerIndexAvg": 11.339463389310689
}

```

collection of portfolio metrics

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|cagrs|object|true|none|none|
|» 1-yr|number(float)|true|none|return over most recent 1-yr period|
|» 3-yr|number(float)|true|none|return over most recent 3-yr period|
|» 5-yr|number(float)|true|none|return over most recent 5-yr period|
|» 10-yr|number(float)|true|none|return over most recent 10-yr period|
|drawDowns|[object]|true|none|list of top-10 drawdowns over requested period|
|» begin|integer(int64)|true|none|unix timestamp of when draw down began|
|» end|integer(int64)|true|none|unix timestamp of when draw down ended|
|» lossPercent|number(float)|true|none|percentage lost during draw down|
|» recovery|number(int64)|true|none|unix timestamp of when portfolio recovered the value it had prior to the draw down|
|sharpeRatio|number(float)|true|none|a measure that indicates the average return minus the risk-free return divided by the standard deviation of return on an investment|
|sortinoRatio|number(float)|true|none|similar to the sharpe ratio but only takes down side risk into account|
|stdDev|number(float)|true|none|standard deviation of returns over time period|
|ulcerIndexAvg|number(float)|true|none|The index increases in value as the price moves farther away from a recent high and falls as the price rises to new highs|

<h2 id="tocS_NullableFloat">NullableFloat</h2>
<!-- backwards compatibility -->
<a id="schemanullablefloat"></a>
<a id="schema_NullableFloat"></a>
<a id="tocSnullablefloat"></a>
<a id="tocsnullablefloat"></a>

```json
{
  "Float64": 0,
  "Valid": true
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|Float64|number|false|none|Field value|
|Valid|boolean|false|none|when false the float value should be considered NaN|

<h2 id="tocS_PingResponse">PingResponse</h2>
<!-- backwards compatibility -->
<a id="schemapingresponse"></a>
<a id="schema_PingResponse"></a>
<a id="tocSpingresponse"></a>
<a id="tocspingresponse"></a>

```json
{
  "message": "API is alive",
  "status": "success",
  "time": "2021-06-19T08:09:10.115924-05:00"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|message|string|true|none|none|
|status|string|true|none|none|
|time|string|true|none|none|

<h2 id="tocS_Portfolio">Portfolio</h2>
<!-- backwards compatibility -->
<a id="schemaportfolio"></a>
<a id="schema_Portfolio"></a>
<a id="tocSportfolio"></a>
<a id="tocsportfolio"></a>

```json
{
  "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
  "name": "Accelerating Dual Momentum",
  "strategy": "adm",
  "arguments": {
    "inTickers": [
      "VFINX",
      "SCZ"
    ],
    "outTicker": "VUSTX"
  },
  "startDate": 315532800,
  "ytdReturn": {
    "Float64": 0,
    "Valid": false
  },
  "cagrSinceInception": {
    "Float64": 0,
    "Valid": false
  },
  "notifications": 4113,
  "created": 1625109105,
  "lastchanged": 1625109339
}

```

a portfolio represents a collection of investments and transactions

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string(uuid)|false|none|unique identifier for the portfolio|
|name|string|true|none|name of the portfolio|
|strategy|string|true|none|shortcode of strategy used with Portfolio|
|arguments|object|true|none|strategy specific arguments|
|startDate|number|true|none|date of first transaction in portfolio as number of seconds since Jan 1, 1970|
|ytdReturn|[NullableFloat](#schemanullablefloat)|false|none|percent return YTD of portfolio|
|cagrSinceInception|[NullableFloat](#schemanullablefloat)|false|none|Compound annual growth rate (CAGR) of portfolio since startDate|
|notifications|number|false|none|integer describing which notifications are enabled for the portoflio|
|created|number|false|none|unix timestamp of when portfolio was created|
|lastChanged|number|false|none|unix timestamp of when portfolio was last modified|

<h2 id="tocS_PortfolioList">PortfolioList</h2>
<!-- backwards compatibility -->
<a id="schemaportfoliolist"></a>
<a id="schema_PortfolioList"></a>
<a id="tocSportfoliolist"></a>
<a id="tocsportfoliolist"></a>

```json
[
  {
    "id": "fa7c7c4d-b00c-40a0-aae2-d5a9f510bf28",
    "name": "Accelerating Dual Momentum",
    "strategy": "adm",
    "arguments": {
      "inTickers": [
        "VFINX",
        "SCZ"
      ],
      "outTicker": "VUSTX"
    },
    "startDate": 315532800,
    "ytdReturn": {
      "Float64": 0,
      "Valid": false
    },
    "cagrSinceInception": {
      "Float64": 0,
      "Valid": false
    },
    "notifications": 4113,
    "created": 1625109105,
    "lastchanged": 1625109339
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[Portfolio](#schemaportfolio)]|false|none|[a portfolio represents a collection of investments and transactions]|

<h2 id="tocS_PortfolioPerformance">PortfolioPerformance</h2>
<!-- backwards compatibility -->
<a id="schemaportfolioperformance"></a>
<a id="schema_PortfolioPerformance"></a>
<a id="tocSportfolioperformance"></a>
<a id="tocsportfolioperformance"></a>

```json
{
  "cagrSinceInception": 0,
  "computedOn": 0,
  "currentAsset": "VFINX",
  "measurements": [
    {
      "justification": {},
      "percentReturn": 0,
      "riskFreeValue": 10000,
      "time": 617846400,
      "value": 10000
    }
  ],
  "metrics": [
    {
      "cagrs": {
        "1-yr": 0.3811364073876089,
        "10-yr": 0.14472186517787256,
        "3-yr": 0.17810447557991638,
        "5-yr": 0.17070350884131757
      },
      "drawDowns": {
        "begin": 1196380800,
        "end": 1235692800,
        "lossPercent": -0.5096921151209781,
        "recovery": 1346371200
      },
      "sharpeRatio": 0.5691295208327964,
      "sortinoRatio": 0.8399589527305837,
      "stdDev": 0.14508276256543967,
      "ulcerIndexAvg": 11.339463389310689
    }
  ],
  "periodEnd": 0,
  "periodStart": 0,
  "totalDeposited": 0,
  "totalWithdrawn": 0,
  "transactions": [
    {
      "date": "1989-07-31T00:00:00Z",
      "justification": {},
      "kind": "DEPOSIT",
      "pricePerShare": 1,
      "shares": 10000,
      "ticker": "$CASH",
      "totalValue": 10000
    }
  ],
  "ytdReturn": 0
}

```

Performance results of a specific simulation

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|cagrSinceInception|number(float)|true|none|compound annual growth rate over whole period|
|computedOn|integer|true|none|when the performance data was calculated|
|currentAsset|string|true|none|asset(s) that are currently held in portfolio, space separated|
|measurements|[object]|true|none|calculation of portfolio value over time|
|» holdings|[Holding](#schemaholding)|true|none|asset(s) that are held in this measurement, space separated example: A AAPL|
|» justification|object|true|none|per strategy specific field with information justifying the list of holdings. E.g. the adm strategy provides an object with each asset's momentum score|
|» percentReturn|number(double)|true|none|percentage return of portfolio at measurement time|
|» riskFreeValue|number(double)|true|none|value of portfolio using risk free rate of return|
|» time|integer(int64)|true|none|time of measurement as a unix timestamp|
|» value|number(double)|true|none|value of portfolio at time|
|metrics|[[MetricBundle](#schemametricbundle)]|true|none|collection of metrics calculated on portfolio|
|periodEnd|integer(int64)|true|none|unix timestamp of the time when the simulation ended|
|periodStart|integer(int64)|true|none|unix timestamp of the time when the simulation began|
|totalDeposited|number(float)|true|none|total deposited in portfolio over simulation period|
|totalWithdrawn|number(float)|true|none|total withdrawn from portfolio over simulation period|
|transactions|[object]|true|none|transactions over simulation period|
|» date|string|true|none|ISO-8601 string of date of transaction|
|» justification|object|true|none|per strategy specific field with information justifying the list of holdings. E.g. the adm strategy provides an object with each asset's momentum score|
|» kind|string|true|none|transaction type|
|» pricePerShare|number(int64)|true|none|price paid per share of the security|
|» ticker|string|true|none|ticker of security invested in|
|» totalValue|number(float)|true|none|total amount of transaction (shares * pricePerShare) + commission|
|» commission|number(float)|false|none|commission paid on transaction|
|ytdReturn|number(float)|true|none|YTD return of portfolio|

#### Enumerated Values

|Property|Value|
|---|---|
|kind|DEPOSIT|
|kind|WITHDRAW|
|kind|MARKER|
|kind|BUY|
|kind|SELL|

<h2 id="tocS_Security">Security</h2>
<!-- backwards compatibility -->
<a id="schemasecurity"></a>
<a id="schema_Security"></a>
<a id="tocSsecurity"></a>
<a id="tocssecurity"></a>

```json
{
  "compositeFigi": "string",
  "cusip": "string",
  "name": "string",
  "ticker": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|compositeFigi|string|false|none|none|
|cusip|string|false|none|none|
|name|string|false|none|none|
|ticker|string|false|none|none|

<h2 id="tocS_Status">Status</h2>
<!-- backwards compatibility -->
<a id="schemastatus"></a>
<a id="schema_Status"></a>
<a id="tocSstatus"></a>
<a id="tocsstatus"></a>

```json
{
  "status": "success"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|status|string|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|status|success|
|status|failure|

<h2 id="tocS_StrategyInfo">StrategyInfo</h2>
<!-- backwards compatibility -->
<a id="schemastrategyinfo"></a>
<a id="schema_StrategyInfo"></a>
<a id="tocSstrategyinfo"></a>
<a id="tocsstrategyinfo"></a>

```json
{
  "name": "string",
  "shortcode": "string",
  "benchmark": "string",
  "description": "string",
  "longDescription": "string",
  "source": "string",
  "version": "1.0.0",
  "arguments": {},
  "suggestedParams": {},
  "metrics": {
    "cagrs": {
      "1-yr": 0.3811364073876089,
      "10-yr": 0.14472186517787256,
      "3-yr": 0.17810447557991638,
      "5-yr": 0.17070350884131757
    },
    "drawDowns": {
      "begin": 1196380800,
      "end": 1235692800,
      "lossPercent": -0.5096921151209781,
      "recovery": 1346371200
    },
    "sharpeRatio": 0.5691295208327964,
    "sortinoRatio": 0.8399589527305837,
    "stdDev": 0.14508276256543967,
    "ulcerIndexAvg": 11.339463389310689
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|name|string|false|none|none|
|shortcode|string|false|none|shortcode of strategy|
|benchmark|string|false|none|preferred benchmark for strategy|
|description|string|false|none|short-description of strategy|
|longDescription|string|false|none|longer description of strategy as markdown|
|source|string|false|none|original author of the strategy|
|version|string|false|none|version number of strategy implementation|
|arguments|object|false|none|strategy specific arguments|
|suggestedParams|object|false|none|optional list of recommended parameters|
|metrics|[MetricBundle](#schemametricbundle)|false|none|collection of portfolio metrics|

<h2 id="tocS_StrategyInfoList">StrategyInfoList</h2>
<!-- backwards compatibility -->
<a id="schemastrategyinfolist"></a>
<a id="schema_StrategyInfoList"></a>
<a id="tocSstrategyinfolist"></a>
<a id="tocsstrategyinfolist"></a>

```json
[
  {
    "name": "string",
    "shortcode": "string",
    "benchmark": "string",
    "description": "string",
    "longDescription": "string",
    "source": "string",
    "version": "1.0.0",
    "arguments": {},
    "suggestedParams": {},
    "metrics": {
      "cagrs": {
        "1-yr": 0.3811364073876089,
        "10-yr": 0.14472186517787256,
        "3-yr": 0.17810447557991638,
        "5-yr": 0.17070350884131757
      },
      "drawDowns": {
        "begin": 1196380800,
        "end": 1235692800,
        "lossPercent": -0.5096921151209781,
        "recovery": 1346371200
      },
      "sharpeRatio": 0.5691295208327964,
      "sortinoRatio": 0.8399589527305837,
      "stdDev": 0.14508276256543967,
      "ulcerIndexAvg": 11.339463389310689
    }
  }
]

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[StrategyInfo](#schemastrategyinfo)]|false|none|none|

