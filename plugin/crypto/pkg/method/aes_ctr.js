const crypto = require('crypto');

const key = Buffer.from('01234567012345670123456701234567', 'utf8');
console.log(key)
const nonce = Buffer.from('0123456701234567', 'utf8');
console.log(nonce)
const ciphertext = Buffer.from([253,72,209,96,36]);

const decipher = crypto.createDecipheriv('aes-256-ctr', key, nonce);
const plaintext = decipher.update(ciphertext);
const finalPlaintext = decipher.final();
console.log(Buffer.concat([plaintext, finalPlaintext]).toString());
