/*
This file is part of secretstream.

secretstream is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

secretstream is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with secretstream.  If not, see <http://www.gnu.org/licenses/>.
*/

var boxes = require('pull-box-stream')
var pull = require('pull-stream')
var toPull = require('stream-to-pull-stream')

pull(
    toPull.source(process.stdin),
    boxes.createUnboxStream(new Buffer(process.argv[2], "base64"), new Buffer(process.argv[3], "base64")),
    toPull.sink(process.stdout)
)
