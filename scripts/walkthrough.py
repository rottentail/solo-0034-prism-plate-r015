#!/usr/bin/env python3
"""Run bounded production HTTP journeys against an isolated ephemeral service."""
import argparse, concurrent.futures, copy, hashlib, json, os, pathlib, signal, socket, struct, subprocess, tempfile, time, urllib.error, urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
def png_alpha(raw):
    """Return width and the alpha byte per pixel of a non-interlaced 8-bit RGBA PNG."""
    import zlib
    assert raw[:8]==b'\x89PNG\r\n\x1a\n'
    pos=8;idat=b''
    while pos<len(raw):
        n=struct.unpack('>I',raw[pos:pos+4])[0];typ=raw[pos+4:pos+8];data=raw[pos+8:pos+8+n];pos+=12+n
        if typ==b'IHDR':
            w,h,bit,ct,_,_,inter=struct.unpack('>IIBBBBB',data);assert (bit,ct,inter)==(8,6,0),(bit,ct,inter)
        elif typ==b'IDAT':idat+=data
        elif typ==b'IEND':break
    buf=zlib.decompress(idat);stride=w*4;rows=[];prev=bytearray(stride);q=0
    for _ in range(h):
        f=buf[q];q+=1;line=bytearray(buf[q:q+stride]);q+=stride
        for i in range(stride):
            a=line[i-4] if i>=4 else 0;b=prev[i];c=prev[i-4] if i>=4 else 0
            if f==1:x=a
            elif f==2:x=b
            elif f==3:x=(a+b)//2
            elif f==4:
                pa,pb,pc=abs(b-c),abs(a-c),abs(a+b-2*c)
                x=a if (pa<=pb and pa<=pc) else (b if pb<=pc else c)
            else:x=0
            line[i]=(line[i]+x)&255
        rows.append(line[3::4]);prev=line
    return w,rows
def main():
    parser=argparse.ArgumentParser();parser.add_argument('--workflow',required=True);args=parser.parse_args()
    seed=json.loads((ROOT/'examples/amber-proof.json').read_text())
    blank=json.loads((ROOT/'examples/fade-mask.json').read_text())
    with tempfile.TemporaryDirectory(prefix='prism-plate-') as tmp:
        base=pathlib.Path(tmp);binary=base/'prismplate';subprocess.run(['go','build','-o',str(binary),'./cmd/prismplate'],cwd=ROOT,check=True)
        sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close();url=f'http://127.0.0.1:{port}'
        log=(base/'service.log').open('w');proc=None
        def start():
            nonlocal proc
            proc=subprocess.Popen([str(binary),'-listen',f'127.0.0.1:{port}','-data',str(base/'state')],stdout=log,stderr=log)
            for _ in range(100):
                if proc.poll() is not None:raise RuntimeError('service exited')
                try:
                    urllib.request.urlopen(url+'/healthz',timeout=1).close();return
                except OSError:time.sleep(.05)
            raise RuntimeError('startup timed out')
        def stop():
            if proc and proc.poll() is None:
                proc.send_signal(signal.SIGTERM)
                try:proc.wait(timeout=45)
                except subprocess.TimeoutExpired:proc.kill();proc.wait();raise
        def call(method,path,data=None,status=200,raw=False,headers=None):
            body=data if isinstance(data,bytes) else (json.dumps(data).encode() if data is not None else None)
            req=urllib.request.Request(url+path,data=body,method=method,headers=headers or {'Content-Type':'application/json'})
            try:
                with urllib.request.urlopen(req,timeout=35) as r:code=r.status;payload=r.read();hs={k.lower():v for k,v in r.headers.items()}
            except urllib.error.HTTPError as e:code=e.code;payload=e.read();hs={k.lower():v for k,v in e.headers.items()}
            if code!=status:raise RuntimeError(f'{method} {path}: expected {status}, observed {code}: {payload[:200]!r}')
            return (payload,hs) if raw else json.loads(payload) if payload else None
        def render(fmt='png',revision=1):return call('POST',f'/v1/plates/amber-plate/render?revision={revision}&format={fmt}',status=201)
        try:
            start();created=call('POST','/v1/plates',seed,status=201);assert created['revisions'][0]['number']==1
            w=args.workflow
            if w=='create':
                assert call('GET','/v1/plates/amber-plate')['title']==seed['title'];call('POST','/v1/plates',seed,status=409)
                bad=copy.deepcopy(seed);bad['id']='bad-plate';bad['document']['layers'][0]['shapes'][0]['transform']=[0]*6;call('POST','/v1/plates',bad,status=400)
            elif w=='revise':
                q={'expected_revision':1,'document':copy.deepcopy(seed['document'])};q['document']['background']['r']=.2
                with concurrent.futures.ThreadPoolExecutor(2) as pool:
                    def attempt(_):
                        try:return call('PUT','/v1/plates/amber-plate',q)
                        except RuntimeError as e:
                            if 'observed 409' in str(e):return 'conflict'
                            raise
                    results=list(pool.map(attempt,range(2)))
                assert sum(x=='conflict' for x in results)==1;assert len(call('GET','/v1/plates/amber-plate')['revisions'])==2
                assert call('GET','/v1/plates/amber-plate')['revisions'][0]['document']==created['revisions'][0]['document']
            elif w=='branch':
                b=call('POST','/v1/plates/amber-plate/branch',{'revision':1,'id':'amber-fork','title':'琥珀新稿'},status=201);assert b['revisions'][0]['digest']==created['revisions'][0]['digest'];call('POST','/v1/plates/amber-plate/branch',{'revision':99,'id':'missing-fork','title':'无效稿'},status=404)
            elif w=='archive':
                call('POST','/v1/plates/amber-plate/archive');call('PUT','/v1/plates/amber-plate',{'expected_revision':1,'document':seed['document']},status=409);assert render()['revision']==1
            elif w in ('png','svg','artifact'):
                fmt='svg' if w=='svg' else 'png';a=render(fmt);raw,hs=call('GET','/v1/artifacts/'+a['id'],raw=True);assert hashlib.sha256(raw).hexdigest()==a['digest']
                if fmt=='png':assert raw[:8]==b'\x89PNG\r\n\x1a\n';assert struct.unpack('>II',raw[16:24])==(192,144)
                else:assert b'<svg' in raw and b'<linearGradient' in raw and b'clipPath' in raw
                assert render(fmt)['digest']==a['digest'];call('GET','/v1/artifacts/'+a['id'],status=304,raw=True,headers={'If-None-Match':hs['etag']});call('GET','/v1/artifacts/missing',status=404)
                if w=='artifact':call('DELETE','/v1/artifacts/'+a['id']);call('GET','/v1/artifacts/'+a['id'],status=409)
            elif w in ('exposure','cancel'):
                job=call('POST','/v1/plates/amber-plate/exposures?revision=1&format=png',status=202)
                if w=='cancel':call('DELETE','/v1/exposures/'+job['id'],status=202)
                for _ in range(200):
                    job=call('GET','/v1/exposures/'+job['id'])
                    if job['state'] in ('succeeded','cancelled','failed'):break
                    time.sleep(.02)
                assert job['state'] in (('succeeded','cancelled') if w=='cancel' else ('succeeded',))
                if job['state']=='succeeded':assert call('GET','/v1/artifacts/'+job['artifact'],raw=True)[0][:8]==b'\x89PNG\r\n\x1a\n'
                call('DELETE','/v1/exposures/absent',status=404)
            elif w=='proof':
                a=render();call('POST','/v1/proofs',{'id':'proof-amber','artifact':a['id']},status=201);assert call('PUT','/v1/proofs/proof-amber',{'state':'accepted','note':'叠色与镂空检查通过'})['state']=='accepted';call('DELETE','/v1/artifacts/'+a['id'],status=409);call('PUT','/v1/proofs/proof-amber',{'state':'rejected','note':'重复裁决'},status=409)
            elif w in ('bundle','restore'):
                a=render();sealed,_=call('GET','/v1/plates/amber-plate/bundle',raw=True);payload=json.loads(sealed);assert payload['plate']['revisions'][0]['digest']==a['scene_digest']
                if w=='bundle':
                    call('POST','/v1/bundles',sealed,status=409);payload['digest']='0'*64;call('POST','/v1/bundles',payload,status=400)
                    stop();os.rename(base/'state',base/'original');start();imported=call('POST','/v1/bundles',sealed,status=201);assert imported['revisions'][0]['digest']==a['scene_digest'];assert render()['digest']==a['digest']
                else:
                    stop();start();assert call('GET','/v1/artifacts/'+a['id'],raw=True)[0][:8]==b'\x89PNG\r\n\x1a\n';assert render()['digest']==a['digest']
            elif w=='fade':
                call('POST','/v1/plates',blank,status=201)
                a=call('POST','/v1/plates/fade-plate/render?revision=1&format=png',status=201)
                raw,_=call('GET','/v1/artifacts/'+a['id'],raw=True)
                assert raw[:8]==b'\x89PNG\r\n\x1a\n' and struct.unpack('>II',raw[16:24])==(160,120)
                s=call('POST','/v1/plates/fade-plate/render?revision=1&format=svg',status=201)
                svg,_=call('GET','/v1/artifacts/'+s['id'],raw=True)
                assert b'<mask' in svg and b'mask-type:alpha' in svg and b'<linearGradient' in svg
                assert call('POST','/v1/plates/fade-plate/render?revision=1&format=png',status=201)['digest']==a['digest']
                # Mask shapes are evaluated in canvas space and content in shape space, so
                # a mask keeps its place when only the target transform moves.
                moved=copy.deepcopy(blank);moved['document']['layers'][0]['shapes'][0]['transform'][4]+=40
                call('PUT','/v1/plates/fade-plate',{'expected_revision':1,'document':moved['document']})
                movedart=call('POST','/v1/plates/fade-plate/render?revision=2&format=png',status=201)
                assert movedart['digest']!=a['digest']
                assert call('GET','/v1/artifacts/'+a['id'],raw=True)[0]==raw
                _,src=png_alpha(raw);_,mv=png_alpha(call('GET','/v1/artifacts/'+movedart['id'],raw=True)[0])
                def wash(x):
                    u=max(0,min(1,((x+0.5)-10)/140))
                    return 1+(0.55-1)*(u/0.7) if u<=0.7 else 0.55+(0.08-0.55)*((u-0.7)/0.3)
                # The content moved but the mask stayed: row 40 is above the band, so
                # alpha must follow the wash at the content's new canvas position.
                assert all(abs(src[40][x]/255-wash(x))<0.02 for x in range(24,57))
                assert all(abs(mv[40][x+40]/255-wash(x+40))<0.02 for x in range(24,57))
                assert any(abs(wash(x+40)-wash(x))>0.1 for x in range(24,57))
                # Inside the two mask shapes' overlap the alpha follows a union, not a product.
                overlap=[(x,y) for x in range(24,34) for y in range(60,66) if abs(src[y][x]/255-(wash(x)+.4-wash(x)*.4))<0.02]
                assert len(overlap)==60,len(overlap)
                assert not any(abs(src[y][x]/255-wash(x)*.4)<0.02 for x,y in overlap)
                # Revision and branch keep the mask editable and on the same revision identity.
                branched=call('POST','/v1/plates/fade-plate/branch',{'revision':2,'id':'fade-fork','title':'蒙版分支'},status=201)
                assert branched['revisions'][0]['document']['layers'][0]['alpha_mask'][1]['transform']==moved['document']['layers'][0]['alpha_mask'][1]['transform']
                bad=copy.deepcopy(blank);bad['id']='fade-invalid';bad['document']['layers'][0]['alpha_mask'][0]['transform']=[0]*6
                call('POST','/v1/plates',bad,status=400)
                bad=copy.deepcopy(blank);bad['id']='fade-duplicate';bad['document']['layers'][0]['alpha_mask'][0]['id']='fade-bar'
                call('POST','/v1/plates',bad,status=400)
                bad=copy.deepcopy(blank);bad['id']='fade-fillrule';bad['document']['layers'][0]['alpha_mask'][0]['fill_rule']='weird'
                call('POST','/v1/plates',bad,status=400)
                # The mask is editable source, so a sealed bundle carries it verbatim.
                sealed,_=call('GET','/v1/plates/fade-plate/bundle',raw=True)
                assert json.loads(sealed)['plate']['revisions'][1]['document']['layers'][0]['alpha_mask'][1]['id']=='fade-band'
                call('POST','/v1/bundles',sealed,status=409)
                # The checked-in transparent-background comparison plate stays renderable.
                compare=json.loads((ROOT/'examples/fade-compare.json').read_text())
                call('POST','/v1/plates',compare,status=201)
                cart=call('POST','/v1/plates/fade-compare/render?revision=1&format=png',status=201)
                craw,_=call('GET','/v1/artifacts/'+cart['id'],raw=True)
                assert struct.unpack('>II',craw[16:24])==(360,140)
            elif w=='isolation':
                lockproc=subprocess.run([str(binary),'-listen','127.0.0.1:0','-data',str(base/'state')],capture_output=True,timeout=3);assert lockproc.returncode!=0;assert b'locked' in lockproc.stderr
                call('POST','/v1/plates',b'{"id":"x"} true',status=400)
            elif w=='rejection':
                bad=copy.deepcopy(seed);bad['id']='oversize';bad['document']['width']=513;call('POST','/v1/plates',bad,status=400);call('GET','/v1/plates/oversize',status=404)
                bad=copy.deepcopy(seed);bad['id']='invalid-brush';bad['document']['layers'][0]['shapes'][0]['brush']['stops'][1]['at']=0;call('POST','/v1/plates',bad,status=400);assert render()['revision']==1
            else:raise RuntimeError('unknown workflow '+w)
            print(json.dumps({'workflow':w,'result':'passed','entry':'HTTP','port':port},ensure_ascii=False))

        finally:stop();log.close()
if __name__=='__main__':main()
